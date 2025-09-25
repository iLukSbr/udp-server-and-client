// Implementa lógica de cliente para transferência confiável sobre UDP,
// incluindo simulação de perdas, NACK e verificação de integridade.
package clientudp

import (
    "errors"
    "fmt"
    "math/rand"
    "net"
    "os"
    "path/filepath"
    "strings"
    "sync/atomic"
    "time"

    "udp/internal/config"
    "udp/internal/protocol"
)

// Política de descarte para simulação de perda de segmentos
type DropPolicy struct {
    rate    float64        // taxa aleatória de descarte (0..1)
    rnd     *rand.Rand     // gerador pseudoaleatório
    dropped map[uint32]struct{} // registra quais seq já foram descartados uma vez
}

// NewDrop cria política de descarte aleatório "single-shot":
// cada seq pode ser descartada no máximo UMA vez. Retransmissões
// do mesmo seq nunca serão descartadas novamente.
func NewDrop(rate float64, seed int64) *DropPolicy {
    if rate <= 0 { return nil }
    return &DropPolicy{rate: rate, rnd: rand.New(rand.NewSource(seed)), dropped: make(map[uint32]struct{})}
}

// ShouldDrop decide se descarta este seq. Regra:
// - Se nil: nunca descarta.
// - Sorteia conforme rate.
// - Se sorteou e ainda não descartou este seq antes: marca e retorna true.
// - Caso contrário false.
func (d *DropPolicy) ShouldDrop(seq uint32) bool {
    if d == nil { return false }
    if d.rate <= 0 { return false }
    if _, already := d.dropped[seq]; already { return false }
    if d.rnd.Float64() < d.rate {
        d.dropped[seq] = struct{}{}
        return true
    }
    return false
}

// Reúne funções de retorno para eventos da transferência.
type Callbacks struct {
    OnMeta     func(protocol.Meta)            // OnMeta é chamado ao receber META
    OnProgress func(bytes uint64, segs uint64) // OnProgress reporta bytes/segmentos acumulados
    OnLog      func(string)                    // OnLog registra mensagens do processo
    OnDone     func(string, bool)              // OnDone informa saída e sucesso de SHA-256
}

// Define parâmetros de uma transferência.
type Config struct {
    Host       string        // Host do servidor
    Port       int           // Porta do servidor
    Path       string        // Caminho do arquivo solicitado no servidor
    Drop       *DropPolicy   // Política de perdas (simulação)
    Timeout    time.Duration // Timeout base para leituras
    Retries    int           // Número de tentativas (timeouts + rounds NACK)
    OutputPath string        // Caminho de saída opcional; se vazio usa recv_<filename>
    Cancel     <-chan struct{} // Canal opcional para cancelamento assíncrono
}

// agrupa os acumuladores e o mapa de recebimento.
type recvState struct {
    recv      map[uint32][]byte // recv armazena payloads recebidos por sequência
    bytesRecv *uint64           // bytesRecv acumula bytes válidos recebidos
    segsRecv  *uint64           // segsRecv conta segmentos válidos recebidos
}

func ctrlType(b []byte) string { return "" }

// Retorna as sequências faltantes dado o total esperado.
func computeMissing(total uint32, recv map[uint32][]byte) []uint32 {
    // missing acumula as sequências não presentes em recv
    missing := make([]uint32, 0) // lista de sequências faltantes
    for i := uint32(0); i < total; i++ { 
        if _, ok := recv[i]; !ok { 
            missing = append(missing, i) 
        }
    }
    return missing
}

// Processa um datagrama recebido, atualizando progresso e
// retornando true se for um EOF.
func processPacket(b []byte, cfg Config, cb Callbacks, recv map[uint32][]byte, bytesRecv *uint64, segsRecv *uint64) (isEOF bool) {
    if protocol.IsCtrl(b) {
        typ, _, err := protocol.DecodeCtrl(b)
        if err == nil && typ == protocol.TypeEOF { return true }
        return false
    }
    // h é o cabeçalho DATA extraído do buffer
        h, err := protocol.UnpackHeader(b[:protocol.HeaderSize()]) // cabeçalho extraído
    if err != nil { return false }
    // payload contém os dados do segmento
        payload := b[protocol.HeaderSize():] // dados do segmento
    
    if len(b) < protocol.HeaderSize() + int(h.Size) {
        if cb.OnLog != nil { 
            cb.OnLog(fmt.Sprintf("ERRO: buffer insuficiente seq=%d: tem %d, precisa %d+%d", 
                h.Seq, len(b), protocol.HeaderSize(), h.Size)) 
        }
        return false 
    }
    
    // Extrair exatamente h.Size bytes como payload
    payload = b[protocol.HeaderSize():protocol.HeaderSize() + int(h.Size)]
    
    if len(payload) != int(h.Size) { 
        if cb.OnLog != nil {
            cb.OnLog(fmt.Sprintf("ERRO: tamanho payload seq=%d: esperado %d, obtido %d", 
                h.Seq, h.Size, len(payload)))
        }
        return false 
    }
    if cfg.Drop != nil && cfg.Drop.ShouldDrop(h.Seq) { if cb.OnLog != nil { cb.OnLog(fmt.Sprintf("DROP seq=%d", h.Seq)) }; return false }
    
    computedCRC32 := protocol.CRC32(payload)
    if computedCRC32 != h.CRC32 { 
        if cb.OnLog != nil {
            cb.OnLog(fmt.Sprintf("ERRO: CRC32 seq=%d: esperado %08X, computado %08X (size=%d)", 
                h.Seq, h.CRC32, computedCRC32, len(payload)))
        }
        return false 
    }
    
    if _, ok := recv[h.Seq]; ok { 
        return false 
    }
    recv[h.Seq] = append([]byte(nil), payload...)
    atomic.AddUint64(bytesRecv, uint64(len(payload)))
    atomic.AddUint64(segsRecv, 1)
    if cb.OnLog != nil && h.Seq % 500 == 0 { cb.OnLog(fmt.Sprintf("STATUS: progresso seq=%d/%d", h.Seq, h.Total-1)) }
    if cb.OnProgress != nil { cb.OnProgress(atomic.LoadUint64(bytesRecv), atomic.LoadUint64(segsRecv)) }
    return false
}

// Envia REQ e aguarda META (ou ERR) com retries.
func sendREQAndGetMeta(conn *net.UDPConn, cfg Config, cb Callbacks) (protocol.Meta, error) {
    // Número de tentativas: primeira + (Retries-1) reenviando.
    attempts := cfg.Retries
    if attempts <= 0 { attempts = 3 }
    if cb.OnLog != nil { cb.OnLog(fmt.Sprintf("STATUS: Solicitando META (até %d tentativas)", attempts)) }
    var meta protocol.Meta
    for try := 1; try <= attempts; try++ {
        if cb.OnLog != nil { cb.OnLog(fmt.Sprintf("STATUS: Enviando REQ tentativa %d/%d", try, attempts)) }
        if _, err := conn.Write(protocol.CtrlREQ(cfg.Path)); err != nil {
            return protocol.Meta{}, err
        }
        _ = conn.SetReadDeadline(time.Now().Add(cfg.Timeout))
        for {
            // Suporte a cancelamento durante espera de META
            if cfg.Cancel != nil {
                select {
                case <-cfg.Cancel:
                    return protocol.Meta{}, errors.New("transferência cancelada")
                default:
                }
            }
            buf := make([]byte, 4096)
            n, _, err := conn.ReadFromUDP(buf)
            if err != nil {
                // Timeout desta tentativa -> sair do loop interno e partir para próxima tentativa
                if cb.OnLog != nil { cb.OnLog(fmt.Sprintf("WARN: Timeout aguardando META (tentativa %d)", try)) }
                break
            }
            if !protocol.IsCtrl(buf[:n]) { continue }
            typ, val, e := protocol.DecodeCtrl(buf[:n])
            if e != nil { continue }
            switch typ {
            case protocol.TypeMETA:
                meta = val.(protocol.Meta)
                if cb.OnMeta != nil { cb.OnMeta(meta) }
                return meta, nil
            case protocol.TypeERR:
                er := val.(protocol.ErrMsg)
                if cb.OnLog != nil { cb.OnLog("ERRO: Servidor respondeu ERR: "+er.Message) }
                return protocol.Meta{}, errors.New(er.Message)
            default:
                // outro controle não esperado => ignora e continua aguardando META / timeout
            }
        }
    }
    return protocol.Meta{}, errors.New("falha ao obter META: tentativas esgotadas")
}

// Lê pacotes até encontrar EOF ou período de inatividade
// após ter recebido algum dado, respeitando o limite maxIdle.
func receiveUntilIdleOrEOF(conn *net.UDPConn, cfg Config, cb Callbacks, st recvState, maxIdle int) (bool, error) {
    // eof indica se EOF foi encontrado
        eof := false       // sinaliza recebimento de EOF
    // idleCount conta timeouts consecutivos
        idleCount := 0     // conta timeouts consecutivos
        if cb.OnLog != nil { cb.OnLog("STATUS: Recebendo dados iniciais") }
    maxIdleIncreased := maxIdle * 3
    for !eof {
        select {
        case <-cfg.Cancel:
            return eof, errors.New("transferência cancelada")
        default:
        }
        // buf armazena o pacote recebido
            buf := make([]byte, protocol.HeaderSize()+config.ChunkSize) // buffer de recepção
        n, _, err := conn.ReadFromUDP(buf)
        if err != nil {
            idleCount++
            if cb.OnLog != nil && idleCount%5 == 0 { // log menos verbose
                cb.OnLog(fmt.Sprintf("Timeout durante recepção inicial (%d/%d)", idleCount, maxIdleIncreased))
            }
            if len(st.recv) > 0 && idleCount >= maxIdleIncreased { // inatividade após algum dado
                    if cb.OnLog != nil { cb.OnLog("STATUS: Ociosidade detectada; iniciando NACK") }
                break
            }
            if idleCount > maxIdleIncreased { return eof, errors.New("timeout aguardando dados iniciais") }
            _ = conn.SetReadDeadline(time.Now().Add(cfg.Timeout))
            continue
        }
        idleCount = 0
        if processPacket(buf[:n], cfg, cb, st.recv, st.bytesRecv, st.segsRecv) { eof = true }
    }
    return eof, nil
}

// Executa rounds de NACK até não restarem faltantes ou esgotar
// maxRounds, processando retransmissões recebidas.
func runNackRounds(conn *net.UDPConn, meta protocol.Meta, cfg Config, cb Callbacks, st recvState, maxRounds int) error {
    // rounds conta quantos NACKs foram enviados
        rounds := 0 // contador de rounds de NACK
    for {
        select {
        case <-cfg.Cancel:
            return errors.New("transferência cancelada")
        default:
        }
        // missing contém as sequências ainda faltantes
        missing := computeMissing(meta.Total, st.recv) // faltantes atuais
        if len(missing) == 0 { return nil }
        if rounds >= maxRounds { 
            if cb.OnLog != nil { 
                cb.OnLog(fmt.Sprintf("ERRO: esgotado retries de NACK; faltando segmentos: %v de total %d", missing, meta.Total)) 
            }
            return errors.New("esgotado retries de NACK; arquivo incompleto") 
        }
        if cb.OnLog != nil { 
            missingDisplay := missing
            if len(missing) > 20 {
                missingDisplay = append(missing[:10], missing[len(missing)-10:]...)
            }
            cb.OnLog(fmt.Sprintf("STATUS: NACK round %d; faltando %d segmentos: %v", rounds+1, len(missing), missingDisplay)) 
        }
        _, _ = conn.Write(protocol.CtrlNACK(missing))
        // Timeout mais longo para retransmissões de arquivos grandes
        timeoutMultiplier := 1 + len(missing)/100 // mais tempo para muitos faltantes
        if timeoutMultiplier > 5 { timeoutMultiplier = 5 }
        extendedTimeout := cfg.Timeout * time.Duration(timeoutMultiplier)
        _ = conn.SetReadDeadline(time.Now().Add(extendedTimeout))
        rounds++
        
        // Processa retransmissões por um período mais longo
        retransmissionReceived := false
        retransmissionDeadline := time.Now().Add(extendedTimeout)
        initialMissingCount := len(missing)
        for time.Now().Before(retransmissionDeadline) {
            select {
            case <-cfg.Cancel:
                return errors.New("transferência cancelada")
            default:
            }
            // buf armazena pacotes retransmitidos de segmentos faltantes
                buf := make([]byte, protocol.HeaderSize()+config.ChunkSize) // buffer de recepção
            n, _, err := conn.ReadFromUDP(buf)
            if err != nil { 
                // Timeout parcial - continua tentando até deadline
                _ = conn.SetReadDeadline(time.Now().Add(cfg.Timeout/4)) // timeouts menores internos
                continue
            }
            if processPacket(buf[:n], cfg, cb, st.recv, st.bytesRecv, st.segsRecv) {
                // EOF recebido - pode continuar ou parar dependendo se ainda faltam
                continue 
            }
            retransmissionReceived = true
            _ = conn.SetReadDeadline(time.Now().Add(cfg.Timeout/4))
        }
        
        // Log do resultado do round
        finalMissingCount := len(computeMissing(meta.Total, st.recv))
        recovered := initialMissingCount - finalMissingCount
        if cb.OnLog != nil {
            if recovered > 0 {
                cb.OnLog(fmt.Sprintf("NACK round %d: recuperados %d segmentos, ainda faltando %d", rounds, recovered, finalMissingCount))
            } else if !retransmissionReceived {
                cb.OnLog(fmt.Sprintf("AVISO: NACK round %d - nenhuma retransmissão recebida", rounds))
            }
        }
    }
}

// Coordena a recepção dos dados, em duas fases: leitura inicial
// até EOF/ociosidade e rounds de NACK.
func receiveData(conn *net.UDPConn, meta protocol.Meta, cfg Config, cb Callbacks) (map[uint32][]byte, error) {
    // recv mapeia sequências para payloads recebidos
        recv := make(map[uint32][]byte) // armazenamento dos payloads por sequência
    // bytesRecv acumula bytes válidos
        var bytesRecv uint64            // total de bytes válidos recebidos
    // segsRecv acumula quantidade de segmentos válidos
        var segsRecv uint64             // total de segmentos válidos recebidos
    // maxRounds define o limite de retries (fallback=3)
        maxRounds := cfg.Retries        // limite de rounds de NACK/timeouts
    if maxRounds <= 0 { maxRounds = 3 }

    st := recvState{recv: recv, bytesRecv: &bytesRecv, segsRecv: &segsRecv}
    if _, err := receiveUntilIdleOrEOF(conn, cfg, cb, st, maxRounds); err != nil {
        return recv, err
    }
    if err := runNackRounds(conn, meta, cfg, cb, st, maxRounds); err != nil {
        return recv, err
    }
    return recv, nil
}

// Reagrupa os chunks, grava o arquivo de saída e valida SHA-256.
func assembleAndVerify(meta protocol.Meta, recv map[uint32][]byte, outputPath string) (string, bool, error) {
    // Verifica se há segmentos faltando
    miss := computeMissing(meta.Total, recv)
    if len(miss) > 0 {
        return "", false, fmt.Errorf("arquivo incompleto: faltam %d segmentos", len(miss))
    }
    // Reconstrói a sequência ordenada para hash/escrita
    chunks := make([][]byte, meta.Total)
    for i := uint32(0); i < meta.Total; i++ { chunks[i] = recv[i] }

    computed := protocol.SHA256FileChunks(chunks)
    match := computed == meta.SHA256

    // Define caminho padrão se não fornecido
    baseOut := outputPath
    if strings.TrimSpace(baseOut) == "" {
        baseOut = "recv_" + filepath.Base(meta.Filename)
    }

    // Se diretório não existir cria (caso inclua subpastas)
    if err := os.MkdirAll(filepath.Dir(baseOut), 0o755); err != nil {
        return "", false, err
    }

    finalPath := baseOut
    // Em caso de mismatch salvamos como .corrupt e retornamos erro para fluxo superior tratar
    var mismatchErr error
    if !match {
        finalPath = baseOut + ".corrupt"
        mismatchErr = fmt.Errorf("sha256 mismatch: esperado %s obtido %s (salvo como %s)", meta.SHA256, computed, filepath.Base(finalPath))
    }

    f, err := os.Create(finalPath)
    if err != nil { return "", false, err }
    defer f.Close()
    for i := uint32(0); i < meta.Total; i++ {
        if _, err := f.Write(chunks[i]); err != nil { return "", false, err }
    }

    if mismatchErr != nil {
        return finalPath, false, mismatchErr
    }
    return finalPath, true, nil
}

// Executa uma transferência da requisição até a verificação.
func transferOnce(cfg Config, cb Callbacks) (string, bool, error) {
	// addr é o endpoint UDP de destino
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)) // resolução do endpoint
	if err != nil { return "", false, err }
	// conn é a conexão UDP usada para a sessão
	conn, err := net.DialUDP("udp", nil, addr) // conexão UDP com o servidor
	if err != nil { return "", false, err }
	defer conn.Close()
	// buffers maiores ajudam a reduzir perdas por estouro de socket
	_ = conn.SetReadBuffer(config.DefaultReadBuffer)
	_ = conn.SetWriteBuffer(config.DefaultWriteBuffer)
	_ = conn.SetReadDeadline(time.Now().Add(cfg.Timeout))

	meta, err := sendREQAndGetMeta(conn, cfg, cb)
	if err != nil { return "", false, err }
	recv, err := receiveData(conn, meta, cfg, cb)
	if err != nil { return "", false, err }
	out, ok, err := assembleAndVerify(meta, recv, cfg.OutputPath)
	return out, ok, err
}

// Inicia a transferência conforme a Config e aciona Callbacks nos eventos.
func RunTransfer(cfg Config, cb Callbacks) {
    out, ok, err := transferOnce(cfg, cb)
    if err != nil && cb.OnLog != nil {
        cb.OnLog("ERRO: " + err.Error())
    }
    if cb.OnLog != nil && strings.TrimSpace(out) != "" {
        if st, statErr := os.Stat(out); statErr == nil {
            cb.OnLog(fmt.Sprintf("STATUS: Arquivo salvo: %s (%d bytes) verificado=%t", out, st.Size(), ok))
        }
    }
    if cb.OnLog != nil {
        cb.OnLog("SUCCESS: Transferência finalizada")
    }
    if cb.OnDone != nil { cb.OnDone(out, ok) }
}

// ListFiles solicita ao servidor a lista de arquivos disponíveis (não recursivo).
func ListFiles(host string, port int, timeout time.Duration) ([]string, error) {
    addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, port))
    if err != nil { return nil, err }
    conn, err := net.DialUDP("udp", nil, addr)
    if err != nil { return nil, err }
    defer conn.Close()
    _ = conn.SetReadDeadline(time.Now().Add(timeout))
    if _, err := conn.Write(protocol.CtrlLIST()); err != nil { return nil, err }
    buf := make([]byte, 4096)
    n, _, err := conn.ReadFromUDP(buf)
    if err != nil { return nil, err }
    if !protocol.IsCtrl(buf[:n]) { return nil, errors.New("resposta não é controle") }
    typ, v, e := protocol.DecodeCtrl(buf[:n])
    if e != nil { return nil, e }
    if typ != protocol.TypeLST { return nil, errors.New("resposta inesperada") }
    lst := v.(protocol.Lst)
    return lst.Names, nil
}
