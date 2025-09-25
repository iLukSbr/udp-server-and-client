package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// implementa um tema personalizado para a aplicação
type CustomTheme struct {
	fyne.Theme
}

// cria um novo tema personalizado
func NewCustomTheme() *CustomTheme {
	return &CustomTheme{
		Theme: theme.DefaultTheme(),
	}
}

// retorna cores personalizadas
func (t *CustomTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary:
		return color.RGBA{R: 0, G: 102, B: 204, A: 255} // Azul mais suave
	case theme.ColorNameSuccess:
		return color.RGBA{R: 0, G: 153, B: 0, A: 255} // Verde sucesso
	case theme.ColorNameWarning:
		return color.RGBA{R: 255, G: 153, B: 0, A: 255} // Laranja aviso
	case theme.ColorNameError:
		return color.RGBA{R: 204, G: 0, B: 0, A: 255} // Vermelho erro
	case theme.ColorNameBackground:
		return color.RGBA{R: 248, G: 249, B: 250, A: 255} // Fundo mais claro
	case theme.ColorNameForeground:
		return color.RGBA{R: 33, G: 37, B: 41, A: 255} // Texto mais escuro
	default:
		return t.Theme.Color(name, variant)
	}
}

// retorna fontes personalizadas
func (t *CustomTheme) Font(style fyne.TextStyle) fyne.Resource {
	// Usa a fonte padrão
	return t.Theme.Font(style)
}

// retorna ícones personalizados
func (t *CustomTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	// Usa os ícones padrão
	return t.Theme.Icon(name)
}

// retorna tamanhos personalizados
func (t *CustomTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameScrollBar:
		return 12
	case theme.SizeNameScrollBarSmall:
		return 8
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameInputRadius:
		return 4
	default:
		return t.Theme.Size(name)
	}
}
