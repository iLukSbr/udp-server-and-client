package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// representa uma barra de status com informações
type StatusBar struct {
	widget.BaseWidget
	statusLabel *widget.Label
	progressBar *widget.ProgressBar
	infoLabel   *widget.Label
}

// cria uma nova barra de status
func NewStatusBar() *StatusBar {
	sb := &StatusBar{
		statusLabel: widget.NewLabel("Pronto"),
		progressBar: widget.NewProgressBar(),
		infoLabel:   widget.NewLabel(""),
	}
	sb.ExtendBaseWidget(sb)
	sb.progressBar.Hide()
	return sb
}

// implementa o widget.CustomWidget
func (sb *StatusBar) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewHBox(
		sb.statusLabel,
		sb.progressBar,
		widget.NewSeparator(),
		sb.infoLabel,
	))
}

// define o status atual
func (sb *StatusBar) SetStatus(status string) {
	sb.statusLabel.SetText(status)
}

// define o progresso (0.0 a 1.0)
func (sb *StatusBar) SetProgress(progress float64) {
	if progress > 0 {
		sb.progressBar.SetValue(progress)
		sb.progressBar.Show()
	} else {
		sb.progressBar.Hide()
	}
}

// define informações adicionais
func (sb *StatusBar) SetInfo(info string) {
	sb.infoLabel.SetText(info)
}

// representa um botão de toolbar com ícone e tooltip
type ToolbarButton struct {
	widget.BaseWidget
	button   *widget.Button
	icon     fyne.Resource
	tooltip  string
	onTapped func()
}

// cria um novo botão de toolbar
func NewToolbarButton(icon fyne.Resource, tooltip string, onTapped func()) *ToolbarButton {
	tb := &ToolbarButton{
		icon:     icon,
		tooltip:  tooltip,
		onTapped: onTapped,
	}
	tb.button = widget.NewButton("", tb.onTapped)
	tb.ExtendBaseWidget(tb)
	return tb
}

// implementa o widget.CustomWidget
func (tb *ToolbarButton) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewButtonRenderer(tb.button)
}

// habilita/desabilita o botão
func (tb *ToolbarButton) SetEnabled(enabled bool) {
	tb.button.SetText(tb.getButtonText(enabled))
}

// retorna o texto do botão baseado no estado
func (tb *ToolbarButton) getButtonText(enabled bool) string {
	if enabled {
		return "●"
	}
	return "○"
}

// representa um campo de entrada com formatação
type FormattedEntry struct {
	widget.Entry
	formatter func(string) string
	validator func(string) error
}

// cria um novo campo de entrada formatado
func NewFormattedEntry(formatter func(string) string, validator func(string) error) *FormattedEntry {
	fe := &FormattedEntry{
		formatter: formatter,
		validator: validator,
	}
	fe.ExtendBaseWidget(fe)
	fe.OnChanged = fe.onTextChanged
	return fe
}

// é chamado quando o texto muda
func (fe *FormattedEntry) onTextChanged(text string) {
	if fe.formatter != nil {
		formatted := fe.formatter(text)
		if formatted != text {
			fe.SetText(formatted)
			// Move o cursor para o final
			fe.CursorColumn = len(formatted)
		}
	}

	if fe.validator != nil {
		if err := fe.validator(text); err != nil {
			// Pode adicionar indicação visual de erro aqui
		}
	}
}

// representa um painel de informações
type InfoPanel struct {
	widget.BaseWidget
	title   *widget.Label
	content *widget.Label
}

// cria um novo painel de informações
func NewInfoPanel(title string) *InfoPanel {
	ip := &InfoPanel{
		title:   widget.NewLabel(title),
		content: widget.NewLabel(""),
	}
	ip.ExtendBaseWidget(ip)
	ip.title.TextStyle.Bold = true
	return ip
}

// implementa o widget.CustomWidget
func (ip *InfoPanel) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewVBox(
		ip.title,
		widget.NewSeparator(),
		ip.content,
	))
}

// define o conteúdo do painel
func (ip *InfoPanel) SetContent(content string) {
	ip.content.SetText(content)
}

// adiciona conteúdo ao painel
func (ip *InfoPanel) AddContent(content string) {
	current := ip.content.Text
	if current == "" {
		ip.content.SetText(content)
	} else {
		ip.content.SetText(current + "\n" + content)
	}
}

// limpa o conteúdo do painel
func (ip *InfoPanel) Clear() {
	ip.content.SetText("")
}

// representa o status de conexão
type ConnectionStatus struct {
	widget.BaseWidget
	statusLabel *widget.Label
	statusIcon  *widget.Label
}

// cria um novo indicador de status de conexão
func NewConnectionStatus() *ConnectionStatus {
	cs := &ConnectionStatus{
		statusLabel: widget.NewLabel("Desconectado"),
		statusIcon:  widget.NewLabel("●"),
	}
	cs.ExtendBaseWidget(cs)
	cs.SetStatus(false)
	return cs
}

// implementa o widget.CustomWidget
func (cs *ConnectionStatus) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewHBox(
		cs.statusIcon,
		cs.statusLabel,
	))
}

// define o status de conexão
func (cs *ConnectionStatus) SetStatus(connected bool) {
	if connected {
		cs.statusLabel.SetText("Conectado")
		cs.statusIcon.SetText("●")
		cs.statusIcon.Importance = widget.SuccessImportance
	} else {
		cs.statusLabel.SetText("Desconectado")
		cs.statusIcon.SetText("●")
		cs.statusIcon.Importance = widget.DangerImportance
	}
}

// representa um indicador de progresso com informações
type ProgressIndicator struct {
	widget.BaseWidget
	progressBar *widget.ProgressBar
	statusLabel *widget.Label
	speedLabel  *widget.Label
	etaLabel    *widget.Label
}

// cria um novo indicador de progresso
func NewProgressIndicator() *ProgressIndicator {
	pi := &ProgressIndicator{
		progressBar: widget.NewProgressBar(),
		statusLabel: widget.NewLabel("Aguardando..."),
		speedLabel:  widget.NewLabel("0 B/s"),
		etaLabel:    widget.NewLabel("--:--"),
	}
	pi.ExtendBaseWidget(pi)
	return pi
}

// implementa o widget.CustomWidget
func (pi *ProgressIndicator) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewVBox(
		pi.statusLabel,
		pi.progressBar,
		container.NewHBox(
			pi.speedLabel,
			widget.NewSeparator(),
			pi.etaLabel,
		),
	))
}

// define o progresso e calcula ETA
func (pi *ProgressIndicator) SetProgress(progress float64, speed float64, totalBytes uint64, receivedBytes uint64) {
	pi.progressBar.SetValue(progress)

	// Atualiza velocidade
	if speed > 0 {
		pi.speedLabel.SetText(formatBytes(speed) + "/s")

		// Calcula ETA
		if speed > 0 && totalBytes > receivedBytes {
			remainingBytes := totalBytes - receivedBytes
			etaSeconds := float64(remainingBytes) / speed
			pi.etaLabel.SetText(formatDuration(etaSeconds))
		} else {
			pi.etaLabel.SetText("--:--")
		}
	} else {
		pi.speedLabel.SetText("0 B/s")
		pi.etaLabel.SetText("--:--")
	}
}

// define o status
func (pi *ProgressIndicator) SetStatus(status string) {
	pi.statusLabel.SetText(status)
}

// formata bytes em unidades legíveis
func formatBytes(bytes float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unit := 0

	for bytes >= 1024 && unit < len(units)-1 {
		bytes /= 1024
		unit++
	}

	if unit == 0 {
		return fmt.Sprintf("%.0f %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.1f %s", bytes, units[unit])
}

// formata duração em formato legível
func formatDuration(seconds float64) string {
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	} else if seconds < 3600 {
		minutes := int(seconds / 60)
		secs := int(seconds) % 60
		return fmt.Sprintf("%02d:%02d", minutes, secs)
	} else {
		hours := int(seconds / 3600)
		minutes := int((seconds - float64(hours*3600)) / 60)
		return fmt.Sprintf("%02d:%02d:00", hours, minutes)
	}
}

// representa um indicador de validação
type ValidationIndicator struct {
	widget.BaseWidget
	icon  *widget.Label
	label *widget.Label
	valid bool
}

// cria um novo indicador de validação
func NewValidationIndicator() *ValidationIndicator {
	vi := &ValidationIndicator{
		icon:  widget.NewLabel("●"),
		label: widget.NewLabel(""),
		valid: false,
	}
	vi.ExtendBaseWidget(vi)
	vi.SetValid(false, "")
	return vi
}

// implementa o widget.CustomWidget
func (vi *ValidationIndicator) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewHBox(
		vi.icon,
		vi.label,
	))
}

// define se o campo é válido
func (vi *ValidationIndicator) SetValid(valid bool, message string) {
	vi.valid = valid
	vi.label.SetText(message)

	if valid {
		vi.icon.SetText("✓")
		vi.icon.Importance = widget.SuccessImportance
	} else {
		vi.icon.SetText("✗")
		vi.icon.Importance = widget.DangerImportance
	}
}

// retorna se o campo é válido
func (vi *ValidationIndicator) IsValid() bool {
	return vi.valid
}

// Helper functions para formatação

// formata um endereço IP
func FormatIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	// Remove caracteres inválidos
	ip = strings.ReplaceAll(ip, " ", "")
	return ip
}

// formata uma porta
func FormatPort(port string) string {
	port = strings.TrimSpace(port)
	if port == "" {
		return ""
	}
	// Remove caracteres não numéricos
	var result strings.Builder
	for _, char := range port {
		if char >= '0' && char <= '9' {
			result.WriteRune(char)
		}
	}
	return result.String()
}

// formata um caminho de arquivo
func FormatFilePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	// Remove caracteres perigosos
	dangerous := []string{"..", "~", "$", "`", "|", "&", ";"}
	for _, char := range dangerous {
		path = strings.ReplaceAll(path, char, "")
	}
	return path
}
