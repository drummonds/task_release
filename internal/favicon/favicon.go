// Package favicon generates simple SVG favicons for static sites.
package favicon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const svgTemplate = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
  <rect width="64" height="64" rx="12" fill="%s"/>
  <text x="32" y="44" font-family="system-ui,sans-serif" font-size="%d" font-weight="bold" fill="white" text-anchor="middle">%s</text>
</svg>
`

// Generate creates a favicon.svg in dir with the given text and background color.
// Text is typically 1-3 character initials. Color is a CSS color (e.g. "#3273dc").
func Generate(dir, text, color string) error {
	if text == "" {
		text = "?"
	}
	if color == "" {
		color = "#3273dc" // Bulma primary blue
	}

	// Scale font size based on text length
	fontSize := 36
	n := utf8.RuneCountInString(text)
	if n == 1 {
		fontSize = 40
	} else if n >= 3 {
		fontSize = 26
	}

	svg := fmt.Sprintf(svgTemplate, color, fontSize, text)

	path := filepath.Join(dir, "favicon.svg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
		return err
	}
	fmt.Printf("Generated %s\n", path)
	return nil
}

// Initials returns up to 2 uppercase initials from a project name.
// Multi-word: "task-plus" -> "TP". Single-word: "lofigui" -> "LG" (first two chars).
// The "go-" prefix is stripped first: "go-postgres" -> "PO".
func Initials(name string) string {
	name = strings.TrimPrefix(name, "go-")
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})

	if len(parts) == 0 || parts[0] == "" {
		return "?"
	}

	// Multi-word: take first letter of each word
	if len(parts) >= 2 {
		var result []rune
		for _, p := range parts {
			if len(p) > 0 {
				r, _ := utf8.DecodeRuneInString(p)
				result = append(result, r)
				if len(result) >= 2 {
					break
				}
			}
		}
		return strings.ToUpper(string(result))
	}

	// Single word: take first two characters
	runes := []rune(parts[0])
	if len(runes) >= 2 {
		return strings.ToUpper(string(runes[:2]))
	}
	return strings.ToUpper(string(runes))
}

// Exists returns true if a favicon file exists in dir (svg, ico, or png).
func Exists(dir string) bool {
	for _, name := range []string{"favicon.svg", "favicon.ico", "favicon.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}
