package logging

import (
    "fmt"
    "image/color"
    "time"

    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/canvas"
    "fyne.io/fyne/v2/container"
)

// LogEntry representa uma linha de log formatada.
type LogEntry struct {
    Level LogLevel
    Text  string
    Time  time.Time
}

// LogView é um visor de logs rolável com cores por nível.
type LogView struct {
    box      *fyne.Container
    scroll   *container.Scroll
    entries  []LogEntry
    maxLines int
}

// NewLogView cria um visor de log responsivo e rolável.
func NewLogView() *LogView {
    box := container.NewVBox()
    scroll := container.NewVScroll(box)
    scroll.SetMinSize(fyne.NewSize(600, 300))
    return &LogView{box: box, scroll: scroll, maxLines: 1000}
}

// CanvasObject retorna o widget para inserir no layout.
func (lv *LogView) CanvasObject() fyne.CanvasObject { return lv.scroll }

// Clear remove todas as linhas.
func (lv *LogView) Clear() {
    lv.entries = nil
    lv.box.Objects = nil
    lv.box.Refresh()
}

// Append adiciona uma nova linha, mantendo limite e fazendo scroll.
func (lv *LogView) Append(level LogLevel, msg string) {
    e := LogEntry{Level: level, Text: msg, Time: time.Now()}
    lv.entries = append(lv.entries, e)
    if len(lv.entries) > lv.maxLines {
        // remove metade antiga para evitar custo de shift frequente
        lv.entries = lv.entries[len(lv.entries)-lv.maxLines/2:]
        // rebuild visual
        lv.box.Objects = nil
        for _, ent := range lv.entries { lv.box.Add(lv.renderEntry(ent)) }
    } else {
        lv.box.Add(lv.renderEntry(e))
    }
    lv.box.Refresh()
    // tenta rolar para baixo (ScrollToBottom disponível nas versões mais novas; fallback manual ignorado)
    if lv.scroll != nil { lv.scroll.ScrollToBottom() }
}

func (lv *LogView) colorFor(level LogLevel) color.Color {
    // Paleta para fundo escuro: INFO branco, WARN amarelo, ERROR vermelho, SUCCESS verde.
    switch level {
    case LogError:
        return color.RGBA{0xFF, 0x55, 0x55, 0xFF}
    case LogWarning:
        return color.RGBA{0xFF, 0xD7, 0x64, 0xFF}
    case LogSuccess:
        return color.RGBA{0x6A, 0xE3, 0x7A, 0xFF} // verde suave
    default: // INFO
        return color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}
    }
}

func (lv *LogView) labelFor(level LogLevel) string {
    switch level {
    case LogError: return "ERROR"
    case LogWarning: return "WARN"
    case LogSuccess: return "SUCCESS"
    default: return "INFO"
    }
}

func (lv *LogView) renderEntry(e LogEntry) fyne.CanvasObject {
    ts := e.Time.Format("15:04:05")
    c := canvas.NewText(fmt.Sprintf("[%s] %s: %s", ts, lv.labelFor(e.Level), e.Text), lv.colorFor(e.Level))
    c.Alignment = fyne.TextAlignLeading
    c.TextSize = 12
    return c
}
