package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"udp/internal/config"
	"udp/internal/protocol"
)

// Servidor UDP linha de comando que atende requisições de transferência de arquivos.
func main() {
	host := flag.String("host", "127.0.0.1", "Host/IP to bind")
	port := flag.Int("port", 19000, "UDP port to bind (>1024)")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", *host, *port))
	if err != nil { fmt.Println("resolve error:", err); os.Exit(1) }
	conn, err := net.ListenUDP("udp", addr)
	if err != nil { fmt.Println("listen error:", err); os.Exit(1) }
	defer conn.Close()
	_ = conn.SetReadBuffer(4 << 20)
	_ = conn.SetWriteBuffer(4 << 20)
	fmt.Printf("CLI UDP server listening on %s:%d\n", *host, *port)

	active := map[string]struct{ meta protocol.Meta; chunks [][]byte }{}

	loadFile := func(path string) (protocol.Meta, [][]byte, error) {
		st, err := os.Stat(path)
		if err != nil { return protocol.Meta{}, nil, err }
		if st.IsDir() { return protocol.Meta{}, nil, fmt.Errorf("is directory") }
		f, err := os.Open(path)
		if err != nil { return protocol.Meta{}, nil, err }
		defer f.Close()
		var chunks [][]byte
		for {
			buf := make([]byte, config.ChunkSize)
			n, err := f.Read(buf)
			if n > 0 { chunks = append(chunks, append([]byte(nil), buf[:n]...)) }
			if err != nil { if err == os.ErrClosed || err.Error() == "EOF" { break }; if err.Error() == "EOF" { break }; if err == nil { continue }; if err.Error() == "EOF" { break }; if err != nil { break } }
			if err != nil { break }
			if n == 0 { break }
		}
		sha := protocol.SHA256FileChunks(chunks)
		meta := protocol.Meta{Filename: filepath.Base(path), Total: uint32(len(chunks)), Size: st.Size(), SHA256: sha, Chunk: config.ChunkSize}
		return meta, chunks, nil
	}

	buf := make([]byte, 4096)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil { continue }
		b := append([]byte(nil), buf[:n]...)
		if !protocol.IsCtrl(b) { continue }
		typ, val, err := protocol.DecodeCtrl(b)
		if err != nil { continue }
		switch typ {
		case protocol.TypeREQ:
			r := val.(protocol.Req)
			safe := filepath.Clean(r.Path)
			if safe == "." || safe == ".." || strings.HasPrefix(safe, "..") {
				conn.WriteToUDP(protocol.CtrlERR("caminho inválido"), addr)
				continue
			}
			abs := filepath.Join(".", safe)
			meta, chunks, err := loadFile(abs)
			if err != nil {
				conn.WriteToUDP(protocol.CtrlERR("arquivo não encontrado"), addr)
				continue
			}
			active[addr.String()] = struct{ meta protocol.Meta; chunks [][]byte }{meta: meta, chunks: chunks}
			conn.WriteToUDP(protocol.CtrlMETA(meta), addr)
			for i, c := range chunks {
				h := protocol.DataHeader{Seq: uint32(i), Total: uint32(len(chunks)), Size: uint16(len(c)), CRC32: protocol.CRC32(c)}
				pkt := append(protocol.PackHeader(h), c...)
				conn.WriteToUDP(pkt, addr)
			}
			conn.WriteToUDP(protocol.CtrlEOF(), addr)
			fmt.Printf("META+DATA+EOF -> %s file=%s total=%d size=%d\n", addr, meta.Filename, meta.Total, meta.Size)
		case protocol.TypeNACK:
			n := val.(protocol.Nack)
			en := active[addr.String()]
			for _, seq := range n.Missing {
				if int(seq) < len(en.chunks) {
					c := en.chunks[int(seq)]
					h := protocol.DataHeader{Seq: seq, Total: uint32(len(en.chunks)), Size: uint16(len(c)), CRC32: protocol.CRC32(c)}
					pkt := append(protocol.PackHeader(h), c...)
					conn.WriteToUDP(pkt, addr)
				}
			}
			fmt.Printf("NACK <- %s missing=%d\n", addr, len(n.Missing))
		case protocol.TypeLIST:
			entries, _ := os.ReadDir(".")
			names := make([]string, 0)
			for _, e := range entries { if !e.IsDir() { names = append(names, e.Name()) } }
			conn.WriteToUDP(protocol.CtrlLST(names), addr)
		}
	}
}
