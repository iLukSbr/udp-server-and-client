// implementa a lógica do servidor de transferência confiável
// sobre UDP, incluindo segmentação, envio de META/DATA/EOF e atendimento a NACKs.
package serverudp

import (
    "errors"
    "fmt"
    "io"
    "net"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "sync/atomic"
    "time"

    "udp/internal/config"
    "udp/internal/protocol"
)

// representa um arquivo segmentado e seus metadados prontos para envio.
type fileEntry struct {
    meta   protocol.Meta // metadados do arquivo
    chunks [][]byte      // segmentos do arquivo
}

// agrega estatísticas de execução do servidor.
type Metrics struct {
    BytesSent       uint64 // total de bytes enviados (inclui headers)
    SegmentsSent    uint64 // quantidade de segmentos iniciais enviados
    NacksReceived   uint64 // quantidade de NACKs recebidos
    Retransmissions uint64 // quantidade de segmentos retransmitidos
    ActiveClients   int64  // estimativa de clientes ativos servidos
}

var (
    activeMu        sync.Mutex              // proteção ao mapa de transfers
    activeTransfers = map[string]*fileEntry{} // associação cliente -> arquivo atual
    mtr             Metrics                 // agregador de métricas do servidor
    srvConn         *net.UDPConn            // socket UDP do servidor
    srvRunning      atomic.Bool             // sinalização de estado de execução
    baseDir         = "."                   // diretório base para servir arquivos
)

// formata representação do cliente para logs
func clientLabel(addr *net.UDPAddr) string {
    if addr == nil { return "client=unknown" }
    return fmt.Sprintf("client=%s:%d", addr.IP.String(), addr.Port)
}

// Retorna uma cópia atômica das métricas atuais.
func Snapshot() Metrics { return Metrics{
    BytesSent: atomic.LoadUint64(&mtr.BytesSent),
    SegmentsSent: atomic.LoadUint64(&mtr.SegmentsSent),
    NacksReceived: atomic.LoadUint64(&mtr.NacksReceived),
    Retransmissions: atomic.LoadUint64(&mtr.Retransmissions),
    ActiveClients: atomic.LoadInt64(&mtr.ActiveClients),
} }

// Carrega e segmenta um arquivo do disco, calculando o SHA-256.
func loadFile(path string) (*fileEntry, error) {
    st, err := os.Stat(path) // estatísticas do arquivo
    if err != nil { return nil, err }
    if st.IsDir() { return nil, errors.New("é diretório") }
    f, err := os.Open(path) // arquivo de entrada
    if err != nil { return nil, err }
    defer f.Close()
    var chunks [][]byte // lista de segmentos lidos
    for {
    buf := make([]byte, config.ChunkSize) // buffer de leitura
    n, err := f.Read(buf)                   // bytes lidos neste ciclo
        if n > 0 { chunks = append(chunks, append([]byte(nil), buf[:n]...)) }
        if err == io.EOF { break }
        if err != nil { return nil, err }
    }
    sha := protocol.SHA256FileChunks(chunks) // hash do arquivo por chunks (Aplicação)
    meta := protocol.Meta{Filename: filepath.Base(path), Total: uint32(len(chunks)), Size: st.Size(), SHA256: sha, Chunk: config.ChunkSize} // Cabeçalho META (Aplicação)
    return &fileEntry{meta: meta, chunks: chunks}, nil
}

// Processa uma requisição de arquivo do cliente, enviando META/DATA/EOF.
func handleREQ(conn *net.UDPConn, addr *net.UDPAddr, req protocol.Req, logAppend func(string)) {
    // Caminho solicitado relativo ao diretório base
    safe := filepath.Clean(req.Path) // caminho sanitizado
    if safe == "." || safe == ".." || strings.HasPrefix(safe, "..") {
        b := protocol.CtrlERR("caminho inválido") // payload de erro compacto
        conn.WriteToUDP(b, addr)
        return
    }
    targetPath := filepath.Join(baseDir, safe) // caminho relativo ao diretório base
    entry, err := loadFile(targetPath)        // arquivo segmentado
    if err != nil {
        b := protocol.CtrlERR("arquivo não encontrado")
        conn.WriteToUDP(b, addr)
        return
    }
    activeMu.Lock(); activeTransfers[addr.String()] = entry; activeMu.Unlock()
    atomic.AddInt64(&mtr.ActiveClients, 1)
    defer atomic.AddInt64(&mtr.ActiveClients, -1)

    // META (controle UC)
    conn.WriteToUDP(protocol.CtrlMETA(entry.meta), addr)
    logAppend(fmt.Sprintf("META -> %s total=%d size=%d", clientLabel(addr), entry.meta.Total, entry.meta.Size))
    for i, chunk := range entry.chunks {
        h := protocol.DataHeader{Seq: uint32(i), Total: uint32(len(entry.chunks)), Size: uint16(len(chunk)), CRC32: protocol.CRC32(chunk)}
        pkt := append(protocol.PackHeader(h), chunk...)
        n, _ := conn.WriteToUDP(pkt, addr)
        atomic.AddUint64(&mtr.BytesSent, uint64(n))
        atomic.AddUint64(&mtr.SegmentsSent, 1)
        time.Sleep(1 * time.Millisecond)
    }
    // EOF (controle UC)
    conn.WriteToUDP(protocol.CtrlEOF(), addr)
    logAppend(fmt.Sprintf("EOF -> %s segmentos=%d", clientLabel(addr), len(entry.chunks)))
}

// Atende pedidos de retransmissão para segmentos listados como faltantes.
func handleNACK(conn *net.UDPConn, addr *net.UDPAddr, nack protocol.Nack) {
    atomic.AddUint64(&mtr.NacksReceived, 1)
    activeMu.Lock(); entry := activeTransfers[addr.String()]; activeMu.Unlock() // busca do arquivo em andamento
    if entry == nil { return }
    for _, seq := range nack.Missing {
        if int(seq) < len(entry.chunks) {
            chunk := entry.chunks[seq]                                                                                          // segmento requerido
            h := protocol.DataHeader{Seq: uint32(seq), Total: uint32(len(entry.chunks)), Size: uint16(len(chunk)), CRC32: protocol.CRC32(chunk)} // cabeçalho de retransmissão
            pkt := append(protocol.PackHeader(h), chunk...)                                                                      // pacote de retransmissão
            n, _ := conn.WriteToUDP(pkt, addr)                                                                                   // bytes reenviados
            atomic.AddUint64(&mtr.BytesSent, uint64(n))
            atomic.AddUint64(&mtr.Retransmissions, 1)
            time.Sleep(0) // cedência de escalonamento
        }
    }
}

// Decodifica uma mensagem de controle (UC) e delega aos handlers.
func dispatchCtrl(conn *net.UDPConn, addr *net.UDPAddr, b []byte, logAppend func(string)) {
    typ, v, err := protocol.DecodeCtrl(b)
    if err != nil { return }
    switch typ {
    case protocol.TypeREQ:
        r := v.(protocol.Req)
        go handleREQ(conn, addr, r, logAppend)
    case protocol.TypeNACK:
        n := v.(protocol.Nack)
    if logAppend != nil { logAppend(fmt.Sprintf("NACK <- %s faltando=%d", clientLabel(addr), len(n.Missing))) }
        go handleNACK(conn, addr, n)
    case protocol.TypeLIST:
        // listar arquivos do diretório base (apenas nomes; não recursivo)
        entries, _ := os.ReadDir(baseDir)
        names := make([]string, 0)
        for _, e := range entries { if !e.IsDir() { names = append(names, e.Name()) } }
        conn.WriteToUDP(protocol.CtrlLST(names), addr)
    }
}

// Executa o loop de leitura de datagramas do servidor.
func packetLoop(conn *net.UDPConn, logAppend func(string)) {
    defer func() { srvRunning.Store(false); conn.Close() }()
    buf := make([]byte, 4096) // buffer de recepção
    for srvRunning.Load() {
        n, addr, err := conn.ReadFromUDP(buf) // leitura do socket
        if err != nil { continue }
        b := append([]byte(nil), buf[:n]...) // cópia do conteúdo recebido
        if protocol.IsCtrl(b) { dispatchCtrl(conn, addr, b, logAppend) }
    }
}

// Configura o diretório base de arquivos a serem servidos (default ".").
func SetBaseDir(dir string) { if strings.TrimSpace(dir) == "" { baseDir = "." } else { baseDir = dir } }

// Inicia o servidor UDP no host/port fornecidos.
func Start(host string, port int, logAppend func(string)) error {
	if srvRunning.Load() { return nil }
	udpAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, port)) // endereço de escuta
	conn, err := net.ListenUDP("udp", udpAddr)                                  // socket de escuta UDP
	if err != nil { return err }
	// buffers maiores ajudam a suportar múltiplos clientes e bursts
	_ = conn.SetReadBuffer(config.DefaultReadBuffer)
	_ = conn.SetWriteBuffer(config.DefaultWriteBuffer)
	srvConn = conn
	srvRunning.Store(true)
	go packetLoop(conn, logAppend)
	return nil
}

// Encerra a execução do servidor UDP.
func Stop() {
    srvRunning.Store(false)
    if srvConn != nil { _ = srvConn.Close() }
}
