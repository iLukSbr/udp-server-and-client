package main

import (
    "fmt"
    "image"
    "image/color"
    "os"
    "path/filepath"
    "runtime"
    "strconv"
    "strings"
    "time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"fyne.io/fyne/v2/theme"

	"udp/internal/clientudp"
	"udp/internal/config"
	"udp/internal/logging"
	"udp/internal/protocol"
)

// Gera imagem simples com barras verticais representando velocidades recentes de transferência.
// Valores são normalizados pelo maior observado e renderizados em tons de azul.
func drawSpark(rates []float64, w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// fundo branco; ajuda a destacar as barras
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.White)
		}
	}
	if len(rates) == 0 || w <= 0 || h <= 0 {
		return img
	}
	max := 0.0
	for _, v := range rates {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		max = 1
	}
	// barras verticais azuis; cada coluna amostra proporcionalmente a série
	n := len(rates)
	for i := 0; i < w; i++ {
		idx := i * n / w
		if idx >= n {
			idx = n - 1
		}
		val := rates[idx]
		bh := int((val / max) * float64(h))
		for y := h - 1; y >= h-bh && y >= 0; y-- {
			img.Set(i, y, color.RGBA{0, 0, 255, 255})
		}
	}
	return img
}

// Interface gráfica do cliente com coleta de parâmetros e iniciação de transferência.
// Exibe progresso, taxa instantânea e logs de eventos durante transferência de arquivos.
func main() {
	// Força driver de renderização por software no Windows se não estiver definido
	if runtime.GOOS == "windows" && strings.TrimSpace(os.Getenv("FYNE_DRIVER")) == "" {
		_ = os.Setenv("FYNE_DRIVER", "software")
	}

	// Carrega configurações salvas
	clientSettings, err := config.LoadClientSettings()
	if err != nil {
		clientSettings = config.DefaultClientSettings()
	}

	a := app.New()                        // instância do app Fyne
	w := a.NewWindow("UDP Client (Fyne)") // janela principal

	hostEntry := widget.NewEntry()
	hostEntry.SetText(clientSettings.Host) // endereço do servidor
	portEntry := widget.NewEntry()
	portEntry.SetText(clientSettings.Port) // porta do servidor
	fileSelect := widget.NewSelectEntry([]string{clientSettings.LastFile})
	fileSelect.SetText(clientSettings.LastFile) // seletor/entrada de arquivo remoto
	outputEntry := widget.NewEntry()
	outputEntry.SetText(clientSettings.OutputPath)
	outputEntry.SetPlaceHolder("caminho ou diretório de saída (ex: C:/tmp ou C:/tmp/arquivo.bin)")
	chooseDirBtn := widget.NewButton("Escolher pasta...", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil { return }
			outputEntry.SetText(uri.Path())
		}, w)
	})

	rateEntry := widget.NewEntry()
	rateEntry.SetText(fmt.Sprintf("%.2f", clientSettings.DropRate)) // probabilidade de descarte
	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetText(clientSettings.Timeout) // duração de ociosidade
	retriesEntry := widget.NewEntry()
	retriesEntry.SetText(fmt.Sprintf("%d", clientSettings.Retries)) // rodadas de NACK

	prog := widget.NewProgressBar()                              // barra de progresso global
	stats := widget.NewLabel("Bytes: 0 | Segs: 0 | Rate: 0 B/s") // resumo numérico
	logView := logging.NewLogView()                              // novo visor de logs rolável/colorido

	// Botões
	var startBtn *widget.Button
	var stopBtn *widget.Button

	// sparkline simples
	var rates []float64 // histórico de taxas instantâneas
	spark := canvas.NewRaster(func(w, h int) image.Image { return drawSpark(rates, w, h) })
	spark.SetMinSize(fyne.NewSize(400, 100))

	var totalBytes uint64 // total a receber, vindo do META
	// acumuladores atômicos para leitura no ticker de UI
	var progBytes uint64 // bytes recebidos (atômico via captura do callback)
	var progSegs uint64  // segmentos recebidos (atômico via captura do callback)
	// estado da UI (apenas lido/atualizado no ticker de UI)
	var lastUIBytes uint64 // último valor visto pela UI
	lastUITick := time.Now()
	var lastRate float64 // taxa calculada (B/s)

	runUI := func(fn func()) { fyne.Do(fn) } // garante execução no thread de UI
	onMeta := func(m protocol.Meta) {
		runUI(func() {
			prog.SetValue(0)
			totalBytes = uint64(m.Size)
			stats.SetText("Bytes: 0 | Segs: 0 | Rate: 0 B/s")
		})
	}
	onProgress := func(b uint64, s uint64) {
		// armazena valores mais recentes para o ticker de UI usar
		progBytes = b
		progSegs = s
	}
	onLog := func(s string) {
		runUI(func() {
			up := strings.ToUpper(s)
			var level logging.LogLevel
			if strings.Contains(up, "ERROR") || strings.Contains(up, "ERRO") {
				level = logging.LogError
			} else if strings.Contains(up, "WARN") || strings.Contains(up, "AVISO") {
				level = logging.LogWarning
			} else if strings.Contains(up, "SUCCESS") || strings.Contains(up, "SUCESSO") || strings.Contains(up, "CONCLUÍDO") || strings.Contains(up, "OK") {
				level = logging.LogSuccess
			} else {
				level = logging.LogInfo
			}
			logView.Append(level, s)
		})
	}
	onDone := func(out string, ok bool) {
		if ok {
			onLog("Concluído: " + out + " (SHA256 OK)")
		} else {
			onLog("Concluído: " + out + " (SHA256 diferente)")
		}
	}

	listBtn := widget.NewButton("Listar arquivos no servidor", func() {
		host := strings.TrimSpace(hostEntry.Text)
		p, _ := strconv.Atoi(strings.TrimSpace(portEntry.Text))
		names, err := clientudp.ListFiles(host, p, 2*time.Second)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		fileSelect.SetOptions(names)
		if len(names) > 0 {
			fileSelect.SetText(names[0])
		}
	})
	var cancelCh chan struct{}
	transferRunning := false
	canceled := false

	// fecha canal somente se ainda aberto (não bloqueia se nil)
	safeClose := func(ch *chan struct{}) {
		if ch == nil || *ch == nil { return }
		// proteção usando recover para qualquer corrida improvável
		defer func(){ _ = recover() }()
		close(*ch)
	}
	startBtn = widget.NewButton("Iniciar", func() { // inicia a transferência em goroutine
		if transferRunning { return }
		// cria novo canal de cancelamento
		cancelCh = make(chan struct{})
		canceled = false
		transferRunning = true
		startBtn.Disable()
		stopBtn.Enable()
		// Valida todos os campos antes de iniciar
		params := config.ValidationParams{
			Host:     hostEntry.Text,
			Port:     portEntry.Text,
			FilePath: fileSelect.Text,
			DropRate: rateEntry.Text,
			Timeout:  timeoutEntry.Text,
			Retries:  retriesEntry.Text,
		}
		errors := config.ValidateAll(params)

		if len(errors) > 0 {
			var errorMsg strings.Builder
			errorMsg.WriteString("Erros de validação:\n")
			for _, err := range errors {
				errorMsg.WriteString(fmt.Sprintf("• %s\n", err.Error()))
			}
			dialog.ShowError(fmt.Errorf(errorMsg.String()), w)
			return
		}

		host := strings.TrimSpace(hostEntry.Text)
		p, _ := strconv.Atoi(strings.TrimSpace(portEntry.Text))
		path := strings.TrimSpace(fileSelect.Text)
		rate, _ := strconv.ParseFloat(strings.TrimSpace(rateEntry.Text), 64)
		seed := time.Now().UnixNano() // seed aleatório interno
		retr, _ := strconv.Atoi(strings.TrimSpace(retriesEntry.Text))
		to, _ := time.ParseDuration(strings.TrimSpace(timeoutEntry.Text))
		dp := clientudp.NewDrop(rate, seed)
		outPath := strings.TrimSpace(outputEntry.Text)
		if outPath == "" {
			outPath = "recv_" + filepath.Base(path)
			if onLog != nil { onLog("Saída não informada; salvando em: " + outPath) }
		} else {
			if st, err := os.Stat(outPath); err == nil && st.IsDir() { // diretório escolhido
				gen := filepath.Join(outPath, "recv_"+filepath.Base(path))
				if onLog != nil { onLog("Diretório selecionado; arquivo será: " + gen) }
				outPath = gen
			}
		}
		cfg := clientudp.Config{Host: host, Port: p, Path: path, Drop: dp, Timeout: to, Retries: retr, OutputPath: outPath, Cancel: cancelCh}
		cbs := clientudp.Callbacks{OnMeta: onMeta, OnProgress: onProgress, OnLog: onLog, OnDone: onDone}
		go func(){
			clientudp.RunTransfer(cfg, cbs)
			runUI(func(){
				transferRunning = false
				cancelCh = nil
				canceled = true
				startBtn.Enable()
				stopBtn.Disable()
			})
		}()
	})
	stopBtn = widget.NewButton("Interromper", func(){
		if !transferRunning || cancelCh == nil || canceled { return }
		canceled = true
		stopBtn.Disable() // evita múltiplos cliques que poderiam chegar antes do estado UI atualizar
		safeClose(&cancelCh)
		cancelCh = nil
		onLog("Solicitado cancelamento da transferência")
	})
	stopBtn.Disable()

	form := widget.NewForm(
		&widget.FormItem{Text: "Host", Widget: hostEntry},
		&widget.FormItem{Text: "Porta", Widget: portEntry},
		&widget.FormItem{Text: "Arquivo", Widget: container.NewBorder(nil, nil, nil, listBtn, fileSelect)},
		&widget.FormItem{Text: "Saída", Widget: container.NewBorder(nil, nil, nil, chooseDirBtn, outputEntry)},
		&widget.FormItem{Text: "Drop rate", Widget: rateEntry},
		&widget.FormItem{Text: "Timeout", Widget: timeoutEntry},
		&widget.FormItem{Text: "Retries", Widget: retriesEntry},
	)
	form.SubmitText = ""
	form.OnSubmit = nil

	// Adiciona ícones aos botões para ficar mais "bonito"
	startBtn.SetIcon(theme.ConfirmIcon())
	stopBtn.SetIcon(theme.CancelIcon())

	buttons := container.NewHBox(startBtn, stopBtn)
	topControls := container.NewVBox(form, buttons)

	// Função para formatar taxa em unidades humanas
	formatRate := func(bps float64) string {
		units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
		u := 0
		for bps >= 1024 && u < len(units)-1 {
			bps /= 1024
			u++
		}
		if bps >= 100 { return fmt.Sprintf("%.0f %s", bps, units[u]) }
		if bps >= 10 { return fmt.Sprintf("%.1f %s", bps, units[u]) }
		return fmt.Sprintf("%.2f %s", bps, units[u])
	}

	metricsSection := container.NewVBox(
		widget.NewLabel("Taxa recente:"),
		spark,
		stats,
	)

	// Área de logs com título - agora com muito mais espaço
	logSection := container.NewBorder(nil, nil, nil, nil,
		container.NewVBox(widget.NewLabel("Logs:"), logView.CanvasObject()),
	)

	// Layout principal usando Border com seção de métricas no topo e logs dominando o centro
	w.SetContent(container.NewBorder(
		container.NewVBox(topControls, metricsSection), // top
		nil, nil, nil,
		logSection,
	))
	// Ticker de UI: atualiza a cada ~200ms, reduzindo carga e evitando concorrência
	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			runUI(func() {
				now := time.Now()
				dt := now.Sub(lastUITick).Seconds()
				if dt <= 0 {
					dt = 1e-6
				}
				b := progBytes
				s := progSegs
				// taxa baseada na diferença desde o último tick
				rate := float64(b-lastUIBytes) / dt
				lastUIBytes = b
				lastUITick = now
				// histórico para sparkline
				if len(rates) > 200 {
					rates = rates[len(rates)-200:]
				}
				rates = append(rates, rate)
				lastRate = rate
				if totalBytes > 0 {
					prog.SetValue(float64(b) / float64(totalBytes))
				}
				stats.SetText(fmt.Sprintf("Bytes: %d | Segs: %d | Rate: %s", b, s, formatRate(lastRate)))
				spark.Refresh()
			})
		}
	}()
	w.Resize(fyne.NewSize(float32(clientSettings.WindowWidth), float32(clientSettings.WindowHeight)))

	// Salva configurações quando a janela for fechada
	w.SetCloseIntercept(func() {
		// Atualiza configurações com valores atuais da UI
		params := config.ClientUIParams{
			Host:       hostEntry.Text,
			Port:       portEntry.Text,
			LastFile:   fileSelect.Text,
			OutputPath: outputEntry.Text,
			Timeout:    timeoutEntry.Text,
			DropRate:   func() float64 { v, _ := strconv.ParseFloat(rateEntry.Text, 64); return v }(),
			Retries:    func() int { v, _ := strconv.Atoi(retriesEntry.Text); return v }(),
		}
		config.UpdateClientSettingsFromUI(clientSettings, params)

		// Salva tamanho da janela
		size := w.Content().Size()
		clientSettings.WindowWidth = int(size.Width)
		clientSettings.WindowHeight = int(size.Height)

		// Salva no arquivo
		if err := config.SaveClientSettings(clientSettings); err != nil {
			fmt.Printf("Erro ao salvar configurações: %v\n", err)
		}

		w.Close()
	})

	w.ShowAndRun()
}
