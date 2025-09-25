package main

import (
    "flag"
    "fmt"
    "math/rand"
    "os"
    "strings"
    "time"

    "udp/internal/clientudp"
    "udp/internal/protocol"
)

func main() {
    target := flag.String("t", "", "Target IP:PORT/file (@ prefix optional)")
    list := flag.Bool("list", false, "List available files on server")
    dropRate := flag.Float64("drop-rate", 0.0, "Random drop rate 0..1 (single-shot per seq)")
    timeout := flag.Duration("timeout", 2*time.Second, "Read timeout (e base para NACK rounds)")
    retries := flag.Int("retries", 5, "Retries for timeouts and NACK rounds")
    out := flag.String("o", "", "Output path (default recv_<filename>)")
    flag.Parse()

    if *target == "" {
        fmt.Println("Usage:")
        fmt.Println("  cli-client -t IP:PORT/file [--drop-rate 0.05 --timeout 2s --retries 5 -o out.bin]")
        fmt.Println("  cli-client -t @IP:PORT/file [--drop-rate 0.05 --timeout 2s --retries 5 -o out.bin]")
        fmt.Println("  cli-client --list IP:PORT")
        os.Exit(2)
    }

    if *list {
        host, port, _, err := protocol.ParseTarget(*target)
        if err != nil { fmt.Println("parse error:", err); os.Exit(1) }
        names, err := clientudp.ListFiles(host, port, *timeout)
        if err != nil { fmt.Println("list error:", err); os.Exit(1) }
        fmt.Printf("Available files on %s:%d:\n", host, port)
        if len(names) == 0 { fmt.Println("  (no files)") } else { for _, n := range names { fmt.Println("  "+n) } }
        return
    }

    host, port, path, err := protocol.ParseTarget(*target)
    if err != nil { fmt.Println("parse error:", err); os.Exit(1) }

    var dp *clientudp.DropPolicy
    if *dropRate > 0 { dp = clientudp.NewDrop(*dropRate, rand.Int63()) }

    cfg := clientudp.Config{Host: host, Port: port, Path: path, Drop: dp, Timeout: *timeout, Retries: *retries, OutputPath: *out}

    var total uint64
    onMeta := func(m protocol.Meta) {
        total = uint64(m.Size)
        fmt.Printf("META: file=%s size=%d total=%d chunk=%d sha256=%s\n", m.Filename, m.Size, m.Total, m.Chunk, m.SHA256)
    }
    var lastBytes uint64
    lastTick := time.Now()
    onProgress := func(b, s uint64) {
        now := time.Now()
        if now.Sub(lastTick) >= 1*time.Second || s == 1 {
            rate := float64(b-lastBytes)/now.Sub(lastTick).Seconds()
            if total > 0 {
                pct := float64(b) * 100 / float64(total)
                fmt.Printf("PROG: %.1f%% bytes=%d segs=%d rate=%.0f B/s\n", pct, b, s, rate)
            } else {
                fmt.Printf("PROG: bytes=%d segs=%d rate=%.0f B/s\n", b, s, rate)
            }
            lastBytes = b; lastTick = now
        }
    }
    onLog := func(s string) { fmt.Println(s) }
    onDone := func(outPath string, ok bool) {
        if strings.TrimSpace(outPath) == "" { outPath = "(no file)" }
        fmt.Printf("DONE: out=%s sha_ok=%t\n", outPath, ok)
    }

    cbs := clientudp.Callbacks{OnMeta: onMeta, OnProgress: onProgress, OnLog: onLog, OnDone: onDone}
    clientudp.RunTransfer(cfg, cbs)
}
