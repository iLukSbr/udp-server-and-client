package logging

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// Níveis de severidade para logs
type LogLevel int

const (
	LogInfo LogLevel = iota
	LogWarning
	LogError
	LogSuccess
)

// Widget Fyne para exibição de logs coloridos
type ColoredLogWidget struct {
	*widget.Entry
	content []string
}

// Cria widget de logs com configuração inicial
func NewColoredLogWidget() *ColoredLogWidget {
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapWord
	entry.Resize(fyne.NewSize(700, 500))
	
	clw := &ColoredLogWidget{
		Entry:   entry,
		content: make([]string, 0),
	}
	clw.Disable()
	return clw
}

// Adiciona entrada de log com timestamp e nível de severidade
func (clw *ColoredLogWidget) Append(level LogLevel, message string) {
	timestamp := time.Now().Format("15:04:05")
	var prefix string

	switch level {
	case LogInfo:
		prefix = "INFO"
	case LogWarning:
		prefix = "WARN"
	case LogError:
		prefix = "ERROR"
	default:
		prefix = "LOG"
	}

	formattedMessage := fmt.Sprintf("[%s] %s: %s", timestamp, prefix, message)
	clw.content = append(clw.content, formattedMessage)

	// Limita o número de linhas para evitar problemas de performance
	if len(clw.content) > 1000 {
		clw.content = clw.content[len(clw.content)-500:]
	}

	// Atualiza o texto
	clw.SetText(strings.Join(clw.content, "\n"))
}

// limpa todos os logs
func (clw *ColoredLogWidget) Clear() {
	clw.content = make([]string, 0)
	clw.SetText("")
}