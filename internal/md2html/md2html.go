// Package md2html converts markdown files to Bulma-styled HTML pages.
package md2html

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/drummonds/task-plus/internal/mdupdate"
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
	File    string // single file to convert (overrides Src directory scan)
}

type breadcrumb struct {
	Label string
	URL   string
}

type pageData struct {
	Title       string
	Project     string
	RootURL     string
	FaviconURL  string
	Content     template.HTML
	Breadcrumbs []breadcrumb
	HasMermaid  bool
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

	// Single file mode
	if cfg.File != "" {
		cfg.Src = filepath.Dir(cfg.File)
		name := filepath.Base(cfg.File)
		if err := convertFile(md, tmpl, cfg, name); err != nil {
			return fmt.Errorf("convert %s: %w", name, err)
		}
		return nil
	}

	// Convert .md files from Src directory.
	entries, err := os.ReadDir(cfg.Src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
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

	// Process auto-markers before goldmark conversion.
	if bytes.Contains(content, []byte("<!-- auto:")) {
		opts := mdupdate.Options{PagesDir: cfg.Dst}
		content = []byte(mdupdate.UpdateContent(string(content), opts))
	}

	title := extractTitle(content, name)

	var buf bytes.Buffer
	if err := md.Convert(content, &buf); err != nil {
		return fmt.Errorf("goldmark: %w", err)
	}

	rendered := replaceMermaidBlocks(buf.String())
	hasMermaid := mermaidBlockRe.MatchString(buf.String())

	outName := strings.TrimSuffix(name, ".md") + ".html"

	rootURL := docsRootURL(cfg.Dst)
	faviconURL := strings.TrimSuffix(rootURL, "index.html") + "favicon.svg"

	// Breadcrumbs: category + page title only (project name is in the navbar)
	var crumbs []breadcrumb
	if cfg.Label != "" {
		crumbs = append(crumbs, breadcrumb{Label: cfg.Label, URL: ""})
	}
	crumbs = append(crumbs, breadcrumb{Label: title, URL: ""})

	data := pageData{
		Title:       title,
		Project:     cfg.Project,
		RootURL:     rootURL,
		FaviconURL:  faviconURL,
		Content:     template.HTML(rendered),
		Breadcrumbs: crumbs,
		HasMermaid:  hasMermaid,
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

// docsRootURL returns the relative URL to index.html from dst.
// Walks up from dst looking for a directory containing index.html or index.md.
func docsRootURL(dst string) string {
	abs, err := filepath.Abs(dst)
	if err != nil {
		return "index.html"
	}
	if hasIndex(abs) {
		return "index.html"
	}
	dir := filepath.Dir(abs)
	for dir != filepath.Dir(dir) {
		if hasIndex(dir) {
			rel, err := filepath.Rel(abs, dir)
			if err != nil {
				return "index.html"
			}
			return filepath.ToSlash(rel) + "/index.html"
		}
		dir = filepath.Dir(dir)
	}
	fmt.Fprintf(os.Stderr, "Warning: no index.html found in %s or parent directories\n", dst)
	return "index.html"
}

func hasIndex(dir string) bool {
	for _, name := range []string{"index.html", "index.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
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

// mermaidBlockRe matches <pre><code class="language-mermaid">...</code></pre> blocks
// produced by goldmark from ```mermaid fenced code.
var mermaidBlockRe = regexp.MustCompile(`(?s)<pre><code class="language-mermaid">(.*?)</code></pre>`)

// htmlEntityDecoder restores HTML entities back to plain text for mermaid.js.
var htmlEntityDecoder = strings.NewReplacer(
	"&gt;", ">", "&lt;", "<", "&amp;", "&", "&quot;", `"`,
)

// replaceMermaidBlocks converts goldmark's mermaid code blocks into
// <pre class="mermaid"> elements for client-side rendering by mermaid.js.
func replaceMermaidBlocks(htmlStr string) string {
	return mermaidBlockRe.ReplaceAllStringFunc(htmlStr, func(match string) string {
		subs := mermaidBlockRe.FindStringSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		src := htmlEntityDecoder.Replace(subs[1])
		return `<pre class="mermaid">` + src + `</pre>`
	})
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
