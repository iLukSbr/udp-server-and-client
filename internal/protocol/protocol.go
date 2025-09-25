// Package protocol define o formato de cabeçalho binário de dados e de controle,
// além de utilitários de checksum/integração usados pelo cliente e servidor.
//
// - Aplicação: este pacote 'protocol' define mensagens de controle (REQ/META/EOF/NACK/ERR)
//   e cabeçalho de dados. A aplicação empacota/desempacota esses formatos.
// - Transporte: UDP (net.DialUDP/ListenUDP). Sem confiabilidade nativa.
// - Rede: IP (endereçamento/roteamento). MTU influencia o tamanho dos datagramas.
// - Enlace: (ex.: Ethernet) impõe MTU tipicamente ~1500 bytes; usou-se chunks de 1024.
package protocol

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"path/filepath"
	"strconv"
	"strings"

	"udp/internal/config"
)

// Parâmetros do protocolo são definidos em internal/config (ChunkSize, ProtocolVersion).

// DATA header layout (network byte order):
// magic(2)='UD', version(1)=1, flags(1)=0, seq(4), total(4), size(2), crc32(4)
var (
	dataMagic = [2]byte{'U', 'D'} // dataMagic contém a assinatura do cabeçalho de dados
)

// representa o cabeçalho binário de um segmento de dados.
type DataHeader struct {
	Seq   uint32 // Seq é o índice do segmento (inicia em 0)
	Total uint32 // Total é a quantidade total de segmentos do arquivo
	Size  uint16 // Size é o tamanho do payload em bytes
	CRC32 uint32 // CRC32 é o checksum do payload (IEEE)
}

// define o tamanho em bytes do cabeçalho binário.
const dataHeaderSize = 2 + 1 + 1 + 4 + 4 + 2 + 4

// Serializa um DataHeader para o formato binário de rede (big-endian).
func PackHeader(h DataHeader) []byte {
	buf := make([]byte, dataHeaderSize) // buf armazena o cabeçalho serializado
	// magic
	buf[0] = dataMagic[0]
	buf[1] = dataMagic[1]
	// version
	buf[2] = byte(config.ProtocolVersion)
	// flags
	buf[3] = 0
	binary.BigEndian.PutUint32(buf[4:8], h.Seq)
	binary.BigEndian.PutUint32(buf[8:12], h.Total)
	binary.BigEndian.PutUint16(buf[12:14], h.Size)
	binary.BigEndian.PutUint32(buf[14:18], h.CRC32)
	return buf
}

// Desserializa o cabeçalho binário em um DataHeader.
func UnpackHeader(b []byte) (DataHeader, error) {
	if len(b) < dataHeaderSize {
		return DataHeader{}, errors.New("buffer curto para header")
	}
	if b[0] != dataMagic[0] || b[1] != dataMagic[1] || b[2] != byte(config.ProtocolVersion) {
		return DataHeader{}, errors.New("header inválido")
	}
	h := DataHeader{}                                // h recebe campos extraídos
	h.Seq = binary.BigEndian.Uint32(b[4:8])         // sequência
	h.Total = binary.BigEndian.Uint32(b[8:12])      // total de segmentos
	h.Size = binary.BigEndian.Uint16(b[12:14])      // tamanho do payload
	h.CRC32 = binary.BigEndian.Uint32(b[14:18])     // checksum CRC32 do payload
	return h, nil
}

// Retorna o tamanho em bytes do cabeçalho DATA.
func HeaderSize() int { return dataHeaderSize }

// Controle binário:
// Header UC v1 (big-endian): magic(2)='UC', version(1)=1, type(1), length(2), payload(variable)
// type: 1=REQ, 2=META, 3=ERR, 4=EOF, 5=NACK
// Payloads:
// - REQ: path UTF-8 (length bytes)
// - META: total(u32) | size(u64) | chunk(u16) | fnLen(u16) | filename(fnLen) | sha256(32 bytes)
// - ERR: code(u16=1) | msgLen(u16) | msg(msgLen)
// - EOF: empty
// - NACK: count(u16) | count * seq(u32)

const (
	TypeREQ  = "REQ"
	TypeMETA = "META"
	TypeERR  = "ERR"
	TypeEOF  = "EOF"
	TypeNACK = "NACK"
	TypeLIST = "LIST" // pedido de listagem de arquivos
	TypeLST  = "LST"  // resposta com lista de arquivos
)

const (
	ctrlMagic0 = 'U'
	ctrlMagic1 = 'C'
)

const (
	ctrlTypeREQ  = 1
	ctrlTypeMETA = 2
	ctrlTypeERR  = 3
	ctrlTypeEOF  = 4
	ctrlTypeNACK = 5
	ctrlTypeLIST = 6
	ctrlTypeLST  = 7
)

type Req struct { Path string }

type Meta struct {
	Filename string
	Total    uint32
	Size     int64
	SHA256   string // 64 hex chars; empacotado/decodificado como 32 bytes binários
	Chunk    int
}

type ErrMsg struct { Message string }

type EOFMsg struct{}

type Nack struct { Missing []uint32 }

type List struct{}

type Lst struct { Names []string } // apenas nomes (UTF-8)

func ctrlHeader(t byte, payloadLen int) []byte {
	b := make([]byte, 2+1+1+2)
	b[0] = ctrlMagic0; b[1] = ctrlMagic1; b[2] = byte(config.ProtocolVersion); b[3] = t
	binary.BigEndian.PutUint16(b[4:6], uint16(payloadLen))
	return b
}

func packREQ(path string) []byte {
	p := []byte(path)
	h := ctrlHeader(ctrlTypeREQ, len(p))
	return append(h, p...)
}

func packMETA(m Meta) []byte {
	fn := []byte(m.Filename)
	sha := parseHexSha(m.SHA256) // 32 bytes
	payload := make([]byte, 4+8+2+2+len(fn)+32)
	binary.BigEndian.PutUint32(payload[0:4], m.Total)
	binary.BigEndian.PutUint64(payload[4:12], uint64(m.Size))
	binary.BigEndian.PutUint16(payload[12:14], uint16(m.Chunk))
	binary.BigEndian.PutUint16(payload[14:16], uint16(len(fn)))
	copy(payload[16:16+len(fn)], fn)
	copy(payload[16+len(fn):], sha)
	h := ctrlHeader(ctrlTypeMETA, len(payload))
	return append(h, payload...)
}

func packERR(msg string) []byte {
	b := []byte(msg)
	payload := make([]byte, 2+2+len(b))
	binary.BigEndian.PutUint16(payload[0:2], 1)
	binary.BigEndian.PutUint16(payload[2:4], uint16(len(b)))
	copy(payload[4:], b)
	h := ctrlHeader(ctrlTypeERR, len(payload))
	return append(h, payload...)
}

func packEOF() []byte { return ctrlHeader(ctrlTypeEOF, 0) }

func packNACK(missing []uint32) []byte {
	payload := make([]byte, 2+4*len(missing))
	binary.BigEndian.PutUint16(payload[0:2], uint16(len(missing)))
	off := 2
	for _, s := range missing {
		binary.BigEndian.PutUint32(payload[off:off+4], s); off += 4
	}
	h := ctrlHeader(ctrlTypeNACK, len(payload))
	return append(h, payload...)
}

func packLIST() []byte { return ctrlHeader(ctrlTypeLIST, 0) }

func packLST(names []string) []byte {
	count := len(names)
	plen := 2
	for _, n := range names { plen += 2 + len([]byte(n)) }
	payload := make([]byte, plen)
	binary.BigEndian.PutUint16(payload[0:2], uint16(count))
	off := 2
	for _, n := range names {
		b := []byte(n)
		binary.BigEndian.PutUint16(payload[off:off+2], uint16(len(b)))
		off += 2
		copy(payload[off:off+len(b)], b)
		off += len(b)
	}
	h := ctrlHeader(ctrlTypeLST, len(payload))
	return append(h, payload...)
}

func parseCtrl(b []byte) (t byte, payload []byte, err error) {
	if len(b) < 6 || b[0] != ctrlMagic0 || b[1] != ctrlMagic1 || b[2] != byte(config.ProtocolVersion) {
		return 0, nil, errors.New("ctrl header inválido")
	}
	t = b[3]
	l := int(binary.BigEndian.Uint16(b[4:6]))
	if len(b) < 6+l { return 0, nil, errors.New("ctrl payload curto") }
	return t, b[6 : 6+l], nil
}

func unpackREQ(p []byte) (Req, error) { return Req{Path: string(p)}, nil }

func unpackMETA(p []byte) (Meta, error) {
	if len(p) < 4+8+2+2+32 { return Meta{}, errors.New("META curto") }
	m := Meta{}
	m.Total = binary.BigEndian.Uint32(p[0:4])
	m.Size = int64(binary.BigEndian.Uint64(p[4:12]))
	m.Chunk = int(binary.BigEndian.Uint16(p[12:14]))
	fnLen := int(binary.BigEndian.Uint16(p[14:16]))
	if len(p) < 16+fnLen+32 { return Meta{}, errors.New("META curto 2") }
	m.Filename = string(p[16 : 16+fnLen])
	m.SHA256 = fmtHash(p[16+fnLen : 16+fnLen+32])
	return m, nil
}

func unpackERR(p []byte) (ErrMsg, error) {
	if len(p) < 4 { return ErrMsg{}, errors.New("ERR curto") }
	ml := int(binary.BigEndian.Uint16(p[2:4]))
	if len(p) < 4+ml { return ErrMsg{}, errors.New("ERR curto 2") }
	return ErrMsg{Message: string(p[4 : 4+ml])}, nil
}

func unpackEOF([]byte) (EOFMsg, error) { return EOFMsg{}, nil }

func unpackNACK(p []byte) (Nack, error) {
	if len(p) < 2 { return Nack{}, errors.New("NACK curto") }
	n := int(binary.BigEndian.Uint16(p[0:2]))
	if len(p) < 2+4*n { return Nack{}, errors.New("NACK curto 2") }
	m := make([]uint32, n)
	off := 2
	for i := 0; i < n; i++ { m[i] = binary.BigEndian.Uint32(p[off : off+4]); off += 4 }
	return Nack{Missing: m}, nil
}

func unpackLST(p []byte) (Lst, error) {
	if len(p) < 2 { return Lst{}, errors.New("LST curto") }
	n := int(binary.BigEndian.Uint16(p[0:2]))
	off := 2
	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if len(p) < off+2 { return Lst{}, errors.New("LST curto 2") }
		l := int(binary.BigEndian.Uint16(p[off : off+2])); off += 2
		if len(p) < off+l { return Lst{}, errors.New("LST curto 3") }
		names = append(names, string(p[off:off+l]))
		off += l
	}
	return Lst{Names: names}, nil
}

// Funções públicas para empacotar mensagens de controle.
func CtrlREQ(path string) []byte              { return packREQ(path) }
func CtrlMETA(m Meta) []byte                  { return packMETA(m) }
func CtrlERR(msg string) []byte               { return packERR(msg) }
func CtrlEOF() []byte                         { return packEOF() }
func CtrlNACK(missing []uint32) []byte        { return packNACK(missing) }
func CtrlLIST() []byte                        { return packLIST() }
func CtrlLST(names []string) []byte           { return packLST(names) }

// Decodifica e informa o tipo como string amigável.
func DecodeCtrl(b []byte) (typ string, v any, err error) {
	t, p, e := parseCtrl(b); if e != nil { return "", nil, e }
	switch t {
	case ctrlTypeREQ:
		q, e := unpackREQ(p); return TypeREQ, q, e
	case ctrlTypeMETA:
		m, e := unpackMETA(p); return TypeMETA, m, e
	case ctrlTypeERR:
		e2, e := unpackERR(p); return TypeERR, e2, e
	case ctrlTypeEOF:
		_, e := unpackEOF(p); return TypeEOF, EOFMsg{}, e
case ctrlTypeNACK:
	nk, e := unpackNACK(p); return TypeNACK, nk, e
case ctrlTypeLIST:
	return TypeLIST, List{}, nil
case ctrlTypeLST:
	lst, e := unpackLST(p); return TypeLST, lst, e
	default:
		return "", nil, errors.New("tipo ctrl desconhecido")
	}
}

// Calcula o checksum IEEE do payload.
func CRC32(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// Calcula o hash SHA-256 de uma sequência de chunks.
func SHA256FileChunks(chunks [][]byte) string {
	h := sha256.New()
	for _, c := range chunks { h.Write(c) }
	return fmtHash(h.Sum(nil))
}

// parseHexSha converte string hex (64) em 32 bytes; se inválido, retorna 32 zeros.
func parseHexSha(s string) []byte {
	b := make([]byte, 32)
	if len(s) != 64 { return b }
	// parse manual simples (sem deps):
	hexval := func(r byte) (byte, bool) {
		switch {
		case r >= '0' && r <= '9': return r - '0', true
		case r >= 'a' && r <= 'f': return r - 'a' + 10, true
		case r >= 'A' && r <= 'F': return r - 'A' + 10, true
		}
		return 0, false
	}
	for i := 0; i < 32; i++ {
		hi, ok1 := hexval(s[i*2]); lo, ok2 := hexval(s[i*2+1])
		if !ok1 || !ok2 { return b }
		b[i] = (hi << 4) | lo
	}
	return b
}

// Converte bytes de hash em string hexadecimal minúscula.
func fmtHash(b []byte) string {
	hex := make([]byte, len(b)*2)      // hex é o buffer resultante em texto
	const hexdigits = "0123456789abcdef" // hexdigits mapeia nibbles para caracteres
	for i, v := range b {
		hex[i*2] = hexdigits[v>>4]
		hex[i*2+1] = hexdigits[v&0x0f]
	}
	return string(hex)
}

// Converte uma string no formato "IP:PORTA/arquivo" ou "@IP:PORTA/arquivo" em host, porta e caminho.
func ParseTarget(target string) (host string, port int, path string, err error) {
	// Remove '@' inicial se presente (opcional para compatibilidade com PowerShell)
	if strings.HasPrefix(target, "@") {
		target = target[1:]
	}
	
	parts := strings.SplitN(target, "/", 2) // separa endpoint e caminho
	if len(parts) != 2 {
		return "", 0, "", errors.New("formato inválido; use IP:PORTA/arquivo ou @IP:PORTA/arquivo")
	}
	ipPort := parts[0] // ipPort contém "IP:PORTA"
	path = parts[1]    // path contém o caminho do arquivo
	hostPort := strings.Split(ipPort, ":")
	if len(hostPort) != 2 {
		return "", 0, "", errors.New("alvo sem porta")
	}
	host = hostPort[0]              // host recebe IP/nome
	p, e := strconv.Atoi(hostPort[1]) // p é a porta convertida
	if e != nil {
		return "", 0, "", e
	}
	port = p // port recebe a porta final
	return
}

// Retorna true se o buffer representar uma mensagem de controle (UC),
// e false se for um pacote de dados (que começa com 'UD').
func IsCtrl(b []byte) bool {
	return len(b) >= 2 && b[0] == 'U' && b[1] == 'C'
}

// Helper para unir caminhos respeitando o SO.
func Join(a, b string) string { return filepath.Join(a, b) }
