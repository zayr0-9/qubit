package tui

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

func renderMessageContentAtWidth(message chatMessage, width int) (string, error) {
	width = max(20, width)
	if message.LocalOnly || message.Role == "status" || message.Role == "error" || message.Role == "reasoning" {
		return wrap(message.Content, width), nil
	}
	markdown, err := renderMarkdown(message.Content, width)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(stripBackgroundANSI(markdown), "\n"), nil
}
func renderMarkdown(markdown string, width int) (string, error) {
	renderWidth := max(20, width)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(noBackgroundMarkdownStyle()),
		glamour.WithWordWrap(renderWidth),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return "", fmt.Errorf("create markdown renderer: %w", err)
	}

	rendered, err := renderer.Render(markdown)
	if err != nil {
		return "", fmt.Errorf("render markdown: %w", err)
	}
	return rendered, nil
}
func stringPtr(value string) *string {
	return &value
}
func boolPtr(value bool) *bool {
	return &value
}
func uintPtr(value uint) *uint {
	return &value
}
func colorToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
func noBackgroundMarkdownStyle() ansi.StyleConfig {
	style := styles.DarkStyleConfig
	style.Document.Margin = uintPtr(0)
	style.Document.Color = stringPtr(colorToHex(text))
	style.Paragraph.Color = stringPtr(colorToHex(text))
	style.Text.Color = stringPtr(colorToHex(text))
	style.Strikethrough.Color = stringPtr(colorToHex(text))
	style.Emph.Color = stringPtr(colorToHex(text))
	style.Strong.Color = stringPtr(colorToHex(text))
	style.HorizontalRule.Color = stringPtr(colorToHex(muted))
	style.Item.Color = stringPtr(colorToHex(text))
	style.Enumeration.Color = stringPtr(colorToHex(muted))
	style.Task.Color = stringPtr(colorToHex(text))
	style.Link.Color = stringPtr(colorToHex(cyan))
	style.LinkText.Color = stringPtr(colorToHex(cyan))
	style.Image.Color = stringPtr(colorToHex(cyan))
	style.ImageText.Color = stringPtr(colorToHex(cyan))
	style.H1.BackgroundColor = nil
	style.H1.Color = stringPtr(colorToHex(accent))
	style.H1.Bold = boolPtr(true)
	style.H2.Color = stringPtr(colorToHex(accent))
	style.H2.Bold = boolPtr(true)
	style.H3.Color = stringPtr(colorToHex(cyan))
	style.H3.Bold = boolPtr(true)
	style.BlockQuote.Color = stringPtr(colorToHex(muted))
	style.Code.Color = stringPtr(colorToHex(cyan))
	style.Code.BackgroundColor = nil
	style.CodeBlock.Color = stringPtr(colorToHex(text))
	style.CodeBlock.Margin = uintPtr(0)
	if style.CodeBlock.Chroma != nil {
		style.CodeBlock.Chroma.Text.Color = stringPtr(colorToHex(text))
		style.CodeBlock.Chroma.Error.BackgroundColor = nil
		style.CodeBlock.Chroma.Background.BackgroundColor = nil
	}
	return style
}
func stripBackgroundANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '\x1b' || i+1 >= len(s) || s[i+1] != '[' {
			b.WriteByte(s[i])
			i++
			continue
		}

		end := i + 2
		for end < len(s) && s[end] != 'm' {
			end++
		}
		if end >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}

		sequence := s[i+2 : end]
		kept := keepNonBackgroundSGR(sequence)
		if len(kept) > 0 {
			b.WriteString("\x1b[")
			b.WriteString(strings.Join(kept, ";"))
			b.WriteByte('m')
		}
		i = end + 1
	}
	return b.String()
}
func keepNonBackgroundSGR(sequence string) []string {
	if sequence == "" {
		return []string{"0"}
	}

	parts := strings.Split(sequence, ";")
	kept := make([]string, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		switch part {
		case "40", "41", "42", "43", "44", "45", "46", "47", "48", "49", "100", "101", "102", "103", "104", "105", "106", "107":
			if part == "48" {
				i = skipExtendedSGR(parts, i)
			}
			continue
		default:
			kept = append(kept, part)
		}
	}
	return kept
}
func skipExtendedSGR(parts []string, i int) int {
	if i+1 >= len(parts) {
		return i
	}
	switch parts[i+1] {
	case "5":
		if i+2 < len(parts) {
			return i + 2
		}
	case "2":
		if i+4 < len(parts) {
			return i + 4
		}
	}
	return i + 1
}
