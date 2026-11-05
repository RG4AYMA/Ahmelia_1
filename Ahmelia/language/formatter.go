package language

import (
	"fmt"
	"strings"
)

// Formatter formatea respuestas en estructura legible
type Formatter struct {
	maxLineLength int
	indent        string
}

// NewFormatter crea un formateador
func NewFormatter() *Formatter {
	return &Formatter{
		maxLineLength: 80,
		indent:        "  ",
	}
}

// FormatAsMarkdown formatea respuesta como Markdown
func (f *Formatter) FormatAsMarkdown(
	title string,
	sections map[string]string,
) string {
	
	parts := []string{
		fmt.Sprintf("# %s", title),
		"",
	}
	
	for sectionName, content := range sections {
		if content != "" {
			parts = append(parts, fmt.Sprintf("## %s", sectionName))
			parts = append(parts, "")
			parts = append(parts, content)
			parts = append(parts, "")
		}
	}
	
	return strings.Join(parts, "\n")
}

// FormatAsBulletList formatea contenido como lista de viñetas
func (f *Formatter) FormatAsBulletList(items []string) string {
	var result []string
	
	for _, item := range items {
		result = append(result, fmt.Sprintf("- %s", item))
	}
	
	return strings.Join(result, "\n")
}

// FormatAsNumberedList formatea contenido como lista numerada
func (f *Formatter) FormatAsNumberedList(items []string) string {
	var result []string
	
	for i, item := range items {
		result = append(result, fmt.Sprintf("%d. %s", i+1, item))
	}
	
	return strings.Join(result, "\n")
}

// FormatAsTable formatea contenido como tabla simple
func (f *Formatter) FormatAsTable(
	headers []string,
	rows [][]string,
) string {
	
	// Calcular ancho de columnas
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}
	
	// Crear tabla
	var result []string
	
	// Header
	headerRow := ""
	separator := ""
	for i, h := range headers {
		headerRow += fmt.Sprintf("%-*s ", colWidths[i], h)
		separator += strings.Repeat("-", colWidths[i]) + "-"
	}
	result = append(result, headerRow)
	result = append(result, separator)
	
	// Rows
	for _, row := range rows {
		dataRow := ""
		for i, cell := range row {
			dataRow += fmt.Sprintf("%-*s ", colWidths[i], cell)
		}
		result = append(result, dataRow)
	}
	
	return strings.Join(result, "\n")
}

// FormatWithEmphasis formatea texto con énfasis
func (f *Formatter) FormatWithEmphasis(text string, emphasizedWords []string) string {
	result := text
	
	for _, word := range emphasizedWords {
		result = strings.ReplaceAll(result, word, fmt.Sprintf("**%s**", word))
	}
	
	return result
}

// FormatParagraphs formatea múltiples párrafos
func (f *Formatter) FormatParagraphs(paragraphs []string) string {
	return strings.Join(paragraphs, "\n\n")
}

// WrapText envuelve texto a cierta longitud de línea
func (f *Formatter) WrapText(text string, maxLength int) string {
	words := strings.Fields(text)
	var result []string
	var currentLine string
	
	for _, word := range words {
		if len(currentLine)+len(word)+1 > maxLength {
			if currentLine != "" {
				result = append(result, currentLine)
				currentLine = word
			} else {
				currentLine = word
			}
		} else {
			if currentLine == "" {
				currentLine = word
			} else {
				currentLine += " " + word
			}
		}
	}
	
	if currentLine != "" {
		result = append(result, currentLine)
	}
	
	return strings.Join(result, "\n")
}

// FormatAsQuote formatea como cita
func (f *Formatter) FormatAsQuote(text string, source string) string {
	result := fmt.Sprintf("> %s", text)
	
	if source != "" {
		result += fmt.Sprintf("\n> — %s", source)
	}
	
	return result
}

// FormatAsCode formatea como bloque de código
func (f *Formatter) FormatAsCode(code string, language string) string {
	return fmt.Sprintf("```%s\n%s\n```", language, code)
}

// FormatAsWarning formatea como advertencia
func (f *Formatter) FormatAsWarning(warning string) string {
	return fmt.Sprintf("⚠️  **Advertencia:** %s", warning)
}

// FormatAsSuccess formatea como éxito
func (f *Formatter) FormatAsSuccess(message string) string {
	return fmt.Sprintf("✅ %s", message)
}

// FormatAsInfo formatea como información
func (f *Formatter) FormatAsInfo(message string) string {
	return fmt.Sprintf("ℹ️  %s", message)
}

// FormatAsQuestion formatea como pregunta
func (f *Formatter) FormatAsQuestion(question string) string {
	return fmt.Sprintf("❓ %s", question)
}

// FormatAsNote formatea como nota
func (f *Formatter) FormatAsNote(note string) string {
	return fmt.Sprintf("📝 %s", note)
}

// FormatAsChecklist formatea como checklist
func (f *Formatter) FormatAsChecklist(items []string, completed []int) string {
	var result []string
	
	for i, item := range items {
		isCompleted := false
		for _, c := range completed {
			if c == i {
				isCompleted = true
				break
			}
		}
		
		if isCompleted {
			result = append(result, fmt.Sprintf("- [x] %s", item))
		} else {
			result = append(result, fmt.Sprintf("- [ ] %s", item))
		}
	}
	
	return strings.Join(result, "\n")
}

// FormatAsTimeline formatea como línea de tiempo
func (f *Formatter) FormatAsTimeline(events []string) string {
	var result []string
	
	for i, event := range events {
		if i == 0 {
			result = append(result, fmt.Sprintf("• %s (inicio)", event))
		} else if i == len(events)-1 {
			result = append(result, fmt.Sprintf("  ↓"))
			result = append(result, fmt.Sprintf("• %s (final)", event))
		} else {
			result = append(result, fmt.Sprintf("  ↓"))
			result = append(result, fmt.Sprintf("• %s", event))
		}
	}
	
	return strings.Join(result, "\n")
}

// FormatProsCons formatea pros y contras
func (f *Formatter) FormatProsCons(pros []string, cons []string) string {
	var result []string
	
	result = append(result, "**Ventajas:**")
	for _, pro := range pros {
		result = append(result, fmt.Sprintf("✓ %s", pro))
	}
	
	result = append(result, "")
	result = append(result, "**Desventajas:**")
	for _, con := range cons {
		result = append(result, fmt.Sprintf("✗ %s", con))
	}
	
	return strings.Join(result, "\n")
}

// FormatComparison formatea comparación
func (f *Formatter) FormatComparison(
	option1Name string,
	option1Features []string,
	option2Name string,
	option2Features []string,
) string {
	
	var result []string
	
	result = append(result, fmt.Sprintf("**%s:**", option1Name))
	for _, feature := range option1Features {
		result = append(result, fmt.Sprintf("- %s", feature))
	}
	
	result = append(result, "")
	result = append(result, fmt.Sprintf("**%s:**", option2Name))
	for _, feature := range option2Features {
		result = append(result, fmt.Sprintf("- %s", feature))
	}
	
	return strings.Join(result, "\n")
}

// FormatScalar formatea valor escalar con barra visual
func (f *Formatter) FormatScalar(label string, value float64, maxValue float64) string {
	barLength := int((value / maxValue) * 20)
	bar := strings.Repeat("█", barLength) + strings.Repeat("░", 20-barLength)
	
	return fmt.Sprintf("%s: [%s] %.0f%%", label, bar, (value/maxValue)*100)
}

// FormatMultipleScalars formatea múltiples valores escalares
func (f *Formatter) FormatMultipleScalars(data map[string]float64) string {
	var result []string
	
	for label, value := range data {
		result = append(result, f.FormatScalar(label, value, 1.0))
	}
	
	return strings.Join(result, "\n")
}

// FormatWithHighlight formatea destacando secciones
func (f *Formatter) FormatWithHighlight(
	text string,
	highlightSections map[string]string,
) string {
	
	result := text
	for section, highlight := range highlightSections {
		result = strings.ReplaceAll(
			result,
			section,
			fmt.Sprintf("**%s**", highlight),
		)
	}
	
	return result
}

// FormatCompact formatea respuesta compacta
func (f *Formatter) FormatCompact(response string) string {
	// Remover espacios en blanco extras
	lines := strings.Split(response, "\n")
	var result []string
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	
	return strings.Join(result, "\n")
}

// FormatExpanded formatea respuesta expandida
func (f *Formatter) FormatExpanded(response string) string {
	// Agregar espacios entre secciones
	lines := strings.Split(response, "\n")
	var result []string
	
	for i, line := range lines {
		result = append(result, line)
		if i < len(lines)-1 && strings.HasPrefix(line, "#") {
			result = append(result, "")
		}
	}
	
	return strings.Join(result, "\n")
}

// FormatResponsive formatea adaptado al dispositivo
func (f *Formatter) FormatResponsive(response string, deviceType string) string {
	switch deviceType {
	case "mobile":
		return f.FormatCompact(response)
	case "desktop":
		return f.FormatExpanded(response)
	default:
		return response
	}
}

// FormatWithIndentation formatea con indentación
func (f *Formatter) FormatWithIndentation(text string, level int) string {
	indent := strings.Repeat(f.indent, level)
	lines := strings.Split(text, "\n")
	
	var result []string
	for _, line := range lines {
		result = append(result, indent+line)
	}
	
	return strings.Join(result, "\n")
}

// FormatAsSummary formatea como resumen ejecutivo
func (f *Formatter) FormatAsSummary(
	title string,
	mainPoints []string,
	keyTakeaway string,
) string {
	
	var result []string
	result = append(result, fmt.Sprintf("## %s", title))
	result = append(result, "")
	
	for _, point := range mainPoints {
		result = append(result, fmt.Sprintf("• %s", point))
	}
	
	result = append(result, "")
	result = append(result, fmt.Sprintf("**Lo más importante:** %s", keyTakeaway))
	
	return strings.Join(result, "\n")
}
