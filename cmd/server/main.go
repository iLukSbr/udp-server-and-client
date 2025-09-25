package main

import (
    "fmt"
    "os"
    "runtime"
    "strconv"
    "strings"
    "time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"udp/internal/config"
	"udp/internal/logging"
	"udp/internal/serverudp"
)

// Interface gráfica do servidor com controles para iniciar/parar listener UDP.
// Atualiza periodicamente métricas e apresenta logs de eventos.
func main() {
	// Força driver de renderização por software no Windows se não estiver definido
	if runtime.GOOS == "windows" && strings.TrimSpace(os.Getenv("FYNE_DRIVER")) == "" {
		_ = os.Setenv("FYNE_DRIVER", "software")
	}

	// Carrega configurações salvas
	serverSettings, err := config.LoadServerSettings()
	if err != nil {
		serverSettings = config.DefaultServerSettings()
	}

	a := app.New()                        // instância do app Fyne
	w := a.NewWindow("UDP Server (Fyne)") // janela principal
	hostEntry := widget.NewEntry()        // endereço de bind
	hostEntry.SetText(serverSettings.Host)
	portEntry := widget.NewEntry() // porta de bind
	portEntry.SetText(serverSettings.Port)
	baseDirEntry := widget.NewEntry() // diretório base de arquivos
	baseDirEntry.SetText(serverSettings.BaseDir)
	status := widget.NewLabel("Parado")                 // estado atual
	bytesLab := widget.NewLabel("Bytes: 0")             // total enviado
	segsLab := widget.NewLabel("Segmentos: 0")          // segmentos enviados
	nacksLab := widget.NewLabel("NACKs: 0")             // NACKs recebidos
	retrLab := widget.NewLabel("Retransm.: 0")          // pacotes retransmitidos
	clientsLab := widget.NewLabel("Clientes ativos: 0") // conectados recentemente
	logView := logging.NewLogView()                     // novo visor de logs coloridos/rolável
	runUI := func(fn func()) { fyne.Do(fn) }            // executa no thread de UI
	logAppend := func(s string) {
		runUI(func() {
			up := strings.ToUpper(s)
			var level logging.LogLevel
			if strings.Contains(up, "ERROR") || strings.Contains(up, "ERRO") {
				level = logging.LogError
			} else if strings.Contains(up, "WARN") || strings.Contains(up, "AVISO") {
				level = logging.LogWarning
			} else if strings.Contains(up, "SUCCESS") || strings.Contains(up, "SUCESSO") || strings.Contains(up, "OK") || strings.Contains(up, "CONCLUÍDO") {
				level = logging.LogSuccess
			} else {
				level = logging.LogInfo
			}
			logView.Append(level, s)
		})
	} // callback de log seguro

	// Seletor de pasta para o diretório base (cliente envia apenas o path relativo)
	pickDirBtn := widget.NewButton("Escolher pasta...", func() {
		d := dialog.NewFolderOpen(func(u fyne.ListableURI, err error) {
			if err != nil || u == nil {
				return
			}
			dir := u.Path()
			baseDirEntry.SetText(dir)
			serverudp.SetBaseDir(strings.TrimSpace(dir))
		}, w)
		d.Show()
	})

	startBtn := widget.NewButton("Iniciar", func() {
		host := hostEntry.Text
		p, _ := strconv.Atoi(strings.TrimSpace(portEntry.Text))
		serverudp.SetBaseDir(strings.TrimSpace(baseDirEntry.Text))
		if err := serverudp.Start(host, p, logAppend); err != nil {
			status.SetText("Erro: " + err.Error())
			return
		}
		status.SetText(fmt.Sprintf("Rodando em %s:%d (base=%s)", host, p, strings.TrimSpace(baseDirEntry.Text)))
	})
	stopBtn := widget.NewButton("Parar", func() {
		serverudp.Stop()
		status.SetText("Parado")
	})

	// Atualizador periódico de métricas (executa updates no thread de UI)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			snap := serverudp.Snapshot()
			runUI(func() {
				bytesLab.SetText(fmt.Sprintf("Bytes: %d", snap.BytesSent))
				segsLab.SetText(fmt.Sprintf("Segmentos: %d", snap.SegmentsSent))
				nacksLab.SetText(fmt.Sprintf("NACKs: %d", snap.NacksReceived))
				retrLab.SetText(fmt.Sprintf("Retransm.: %d", snap.Retransmissions))
				clientsLab.SetText(fmt.Sprintf("Clientes ativos: %d", snap.ActiveClients))
			})
		}
	}()

    // Form para alinhamento limpo
    form := widget.NewForm(
        &widget.FormItem{Text: "Host", Widget: hostEntry},
        &widget.FormItem{Text: "Porta", Widget: portEntry},
        &widget.FormItem{Text: "Diretório base", Widget: container.NewBorder(nil, nil, nil, pickDirBtn, baseDirEntry)},
    )
    buttons := container.NewHBox(startBtn, stopBtn)
    metrics := container.NewGridWithColumns(2,
        container.NewVBox(bytesLab, segsLab),
        container.NewVBox(nacksLab, retrLab),
    )
    statsBox := container.NewVBox(status, metrics, clientsLab, widget.NewLabel("Logs:"))
    top := container.NewVBox(form, buttons, statsBox)
    w.SetContent(container.NewBorder(top, nil, nil, nil, logView.CanvasObject()))
	w.Resize(fyne.NewSize(float32(serverSettings.WindowWidth), float32(serverSettings.WindowHeight)))

	// Salva configurações quando a janela for fechada
	w.SetCloseIntercept(func() {
		// Atualiza configurações com valores atuais da UI
		config.UpdateServerSettingsFromUI(
			serverSettings,
			hostEntry.Text,
			portEntry.Text,
			baseDirEntry.Text,
		)

		// Salva tamanho da janela
		size := w.Content().Size()
		serverSettings.WindowWidth = int(size.Width)
		serverSettings.WindowHeight = int(size.Height)

		// Salva no arquivo
		if err := config.SaveServerSettings(serverSettings); err != nil {
			fmt.Printf("Erro ao salvar configurações: %v\n", err)
		}

		w.Close()
	})

	w.ShowAndRun()
}
