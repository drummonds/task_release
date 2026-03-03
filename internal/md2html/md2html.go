// Package md2html converts markdown files to Bulma-styled HTML pages.
package md2html

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

//go:embed template.html
var templateFS embed.FS

// Config holds the parameters for a conversion run.
type Config struct {
	Src     string // source markdown directory
	Dst     string // destination HTML directory
	Label   string // breadcrumb label for this doc set
	Project string // project name for breadcrumb root
}

type breadcrumb struct {
	Label string
	URL   string
}

type pageData struct {
	Title       string
	Project     string
	Content     template.HTML
	Breadcrumbs []breadcrumb
}

// Run converts all .md files in Src to .html files in Dst.
func Run(cfg Config) error {
	if cfg.Project == "" {
		cfg.Project = detectProject()
	}

	tmplBytes, err := templateFS.ReadFile("template.html")
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}
	tmpl, err := template.New("page").Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	if err := os.MkdirAll(cfg.Dst, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", cfg.Dst, err)
	}

	entries, err := os.ReadDir(cfg.Src)
	if err != nil {
		return fmt.Errorf("read %s: %w", cfg.Src, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if err := convertFile(md, tmpl, cfg, entry.Name()); err != nil {
			return fmt.Errorf("convert %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func convertFile(md goldmark.Markdown, tmpl *template.Template, cfg Config, name string) error {
	content, err := os.ReadFile(filepath.Join(cfg.Src, name))
	if err != nil {
		return err
	}

	title := extractTitle(content, name)

	var buf bytes.Buffer
	if err := md.Convert(content, &buf); err != nil {
		return fmt.Errorf("goldmark: %w", err)
	}

	outName := strings.TrimSuffix(name, ".md") + ".html"
	data := pageData{
		Title:   title,
		Project: cfg.Project,
		Content: template.HTML(buf.String()),
		Breadcrumbs: []breadcrumb{
			{Label: cfg.Project, URL: "../index.html"},
			{Label: cfg.Label, URL: ""},
			{Label: title, URL: ""},
		},
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return fmt.Errorf("template: %w", err)
	}

	outPath := filepath.Join(cfg.Dst, outName)
	if err := os.WriteFile(outPath, out.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s -> %s\n", filepath.Join(cfg.Src, name), outPath)
	return nil
}

func extractTitle(content []byte, fallback string) string {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return strings.TrimSuffix(fallback, ".md")
}

// detectProject parses go.mod in CWD to extract the last path element of the module name.
func detectProject() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "project"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			mod := strings.TrimPrefix(line, "module ")
			mod = strings.TrimSpace(mod)
			parts := strings.Split(mod, "/")
			return parts[len(parts)-1]
		}
	}
	return "project"
}
