// Package md2html converts markdown files to Bulma-styled HTML pages.
package md2html

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/drummonds/task-plus/internal/git"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

//go:embed template.html index.html
var templateFS embed.FS

// Config holds the parameters for a conversion run.
type Config struct {
	Src      string // source markdown directory
	Dst      string // destination HTML directory
	Label    string // breadcrumb label for this doc set
	Project  string // project name for breadcrumb root
	File     string // single file to convert (overrides Src directory scan)
	Index    bool   // generate index.html listing all pages
	Subtitle string // subtitle for the index page
}

// pageInfo holds metadata about a converted page for the index.
type pageInfo struct {
	Title    string
	Filename string
}

// LinkInfo holds a label/URL pair for the index links section.
type LinkInfo struct {
	Label string
	URL   string
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
	HasMermaid  bool
}

// Run converts all .md files in Src to .html files in Dst.
func Run(cfg Config) error {
	if cfg.Project == "" {
		cfg.Project = detectProject()
	}
	if cfg.Subtitle == "" {
		cfg.Subtitle = "Documentation"
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
		if _, err := convertFile(md, tmpl, cfg, name); err != nil {
			return fmt.Errorf("convert %s: %w", name, err)
		}
		return nil
	}

	// Convert .md files from Src directory.
	converted := make(map[string]pageInfo) // filename -> info
	entries, err := os.ReadDir(cfg.Src)
	if err != nil && !cfg.Index {
		// If --index only (no src to read), that's fine.
		return fmt.Errorf("read %s: %w", cfg.Src, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "_index.md" {
			continue
		}
		info, err := convertFile(md, tmpl, cfg, entry.Name())
		if err != nil {
			return fmt.Errorf("convert %s: %w", entry.Name(), err)
		}
		converted[info.Filename] = info
	}

	if cfg.Index {
		// Scan --dst for all .html files, merging with just-converted pages.
		pages := scanDstPages(cfg.Dst, converted)
		links := discoverLinks()
		if err := generateIndex(md, cfg, pages, links); err != nil {
			return fmt.Errorf("generate index: %w", err)
		}
	}

	return nil
}

func convertFile(md goldmark.Markdown, tmpl *template.Template, cfg Config, name string) (pageInfo, error) {
	content, err := os.ReadFile(filepath.Join(cfg.Src, name))
	if err != nil {
		return pageInfo{}, err
	}

	title := extractTitle(content, name)

	var buf bytes.Buffer
	if err := md.Convert(content, &buf); err != nil {
		return pageInfo{}, fmt.Errorf("goldmark: %w", err)
	}

	rendered := replaceMermaidBlocks(buf.String())
	hasMermaid := mermaidBlockRe.MatchString(buf.String())

	outName := strings.TrimSuffix(name, ".md") + ".html"

	// The output file always lands in cfg.Dst alongside index.html,
	// so the breadcrumb root is always a sibling link.
	rootURL := "index.html"

	data := pageData{
		Title:   title,
		Project: cfg.Project,
		Content: template.HTML(rendered),
		Breadcrumbs: []breadcrumb{
			{Label: cfg.Project, URL: rootURL},
			{Label: cfg.Label, URL: ""},
			{Label: title, URL: ""},
		},
		HasMermaid: hasMermaid,
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return pageInfo{}, fmt.Errorf("template: %w", err)
	}

	outPath := filepath.Join(cfg.Dst, outName)
	if err := os.WriteFile(outPath, out.Bytes(), 0o644); err != nil {
		return pageInfo{}, err
	}
	fmt.Printf("%s -> %s\n", filepath.Join(cfg.Src, name), outPath)
	return pageInfo{Title: title, Filename: outName}, nil
}

// indexData holds the template data for the index page.
type indexData struct {
	Project  string
	Subtitle string
	Intro    template.HTML
	Pages    []pageInfo
	Links    []LinkInfo
}

// generateIndex creates an index.html listing all converted pages.
func generateIndex(md goldmark.Markdown, cfg Config, pages []pageInfo, links []LinkInfo) error {
	idxBytes, err := templateFS.ReadFile("index.html")
	if err != nil {
		return fmt.Errorf("read index template: %w", err)
	}
	idxTmpl, err := template.New("index").Parse(string(idxBytes))
	if err != nil {
		return fmt.Errorf("parse index template: %w", err)
	}

	// Render optional _index.md intro content.
	// Look in both --dst and --src (--dst first, since that's where docs live).
	var intro template.HTML
	for _, dir := range []string{cfg.Dst, cfg.Src} {
		introPath := filepath.Join(dir, "_index.md")
		if introContent, err := os.ReadFile(introPath); err == nil {
			var buf bytes.Buffer
			if err := md.Convert(introContent, &buf); err == nil {
				intro = template.HTML(buf.String())
			}
			break
		}
	}

	data := indexData{
		Project:  cfg.Project,
		Subtitle: cfg.Subtitle,
		Intro:    intro,
		Pages:    pages,
		Links:    links,
	}

	var out bytes.Buffer
	if err := idxTmpl.Execute(&out, data); err != nil {
		return fmt.Errorf("template: %w", err)
	}

	outPath := filepath.Join(cfg.Dst, "index.html")
	if err := os.WriteFile(outPath, out.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Printf("index -> %s\n", outPath)
	return nil
}

// scanDstPages scans the destination directory for .html files, merging with
// already-converted pages. Converted page titles take precedence.
func scanDstPages(dst string, converted map[string]pageInfo) []pageInfo {
	entries, err := os.ReadDir(dst)
	if err != nil {
		// Return just the converted pages if dst can't be read.
		pages := make([]pageInfo, 0, len(converted))
		for _, p := range converted {
			pages = append(pages, p)
		}
		return pages
	}

	seen := make(map[string]bool)
	var pages []pageInfo

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".html") || name == "index.html" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true

		// Prefer title from conversion (extracted from markdown heading).
		if info, ok := converted[name]; ok {
			pages = append(pages, info)
			continue
		}
		// Extract title from existing .html file's <title> tag.
		title := extractHTMLTitle(filepath.Join(dst, name), name)
		pages = append(pages, pageInfo{Title: title, Filename: name})
	}

	sort.Slice(pages, func(i, j int) bool {
		return pages[i].Filename < pages[j].Filename
	})
	return pages
}

// titleTagRe matches <title>...</title> in HTML.
var titleTagRe = regexp.MustCompile(`<title>([^<]+)</title>`)

// extractHTMLTitle reads an HTML file and extracts the page title from <title>.
// The template format is "Title - Project", so we strip the " - Project" suffix.
func extractHTMLTitle(path, fallback string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return strings.TrimSuffix(fallback, ".html")
	}
	m := titleTagRe.FindSubmatch(data)
	if m == nil {
		return strings.TrimSuffix(fallback, ".html")
	}
	title := string(m[1])
	// Strip " - ProjectName" suffix added by template.
	if idx := strings.Index(title, " - "); idx > 0 {
		title = title[:idx]
	}
	return title
}

// discoverLinks auto-discovers project links from git remotes and task-plus.yml.
// When run from a docs repo (has parent_repo), shows both parent "Source" links
// and current "Docs repo" links. When run from a non-docs repo, shows "Source" links only.
// Also adds a "Documentation" link from statichost config if available.
func discoverLinks() []LinkInfo {
	var links []LinkInfo

	cwdRemotes := gitRemoteURLs(".")
	parentDir := readParentRepo(".")
	isDocsRepo := parentDir != ""

	// Documentation link from statichost config
	if docURL := readDocumentationURL("."); docURL != "" {
		links = append(links, LinkInfo{Label: "Documentation", URL: docURL})
	}

	if isDocsRepo {
		// We're in a docs repo: parent remotes are "Source", cwd remotes are "Docs repo".
		parentRemotes := gitRemoteURLs(parentDir)
		for _, name := range sortedKeys(parentRemotes) {
			webURL := git.URLToWeb(parentRemotes[name])
			if webURL == "" {
				continue
			}
			label := "Source"
			if len(parentRemotes) > 1 {
				label = "Source (" + name + ")"
			}
			links = append(links, LinkInfo{Label: label, URL: webURL})
		}
		for _, name := range sortedKeys(cwdRemotes) {
			webURL := git.URLToWeb(cwdRemotes[name])
			if webURL == "" {
				continue
			}
			label := "Docs repo"
			if len(cwdRemotes) > 1 {
				label = "Docs repo (" + name + ")"
			}
			links = append(links, LinkInfo{Label: label, URL: webURL})
		}
	} else {
		// Not a docs repo: cwd remotes are "Source".
		for _, name := range sortedKeys(cwdRemotes) {
			webURL := git.URLToWeb(cwdRemotes[name])
			if webURL == "" {
				continue
			}
			label := "Source"
			if len(cwdRemotes) > 1 {
				label = "Source (" + name + ")"
			}
			links = append(links, LinkInfo{Label: label, URL: webURL})
		}
	}

	return links
}

// readParentRepo reads parent_repo from task-plus.yml in dir.
func readParentRepo(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "task-plus.yml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "parent_repo:") {
			val := strings.TrimPrefix(line, "parent_repo:")
			val = strings.TrimSpace(val)
			if val != "" && !filepath.IsAbs(val) {
				val = filepath.Join(dir, val)
			}
			return val
		}
	}
	return ""
}

// gitRemoteURLs returns a map of remote name -> URL for the git repo in dir.
func gitRemoteURLs(dir string) map[string]string {
	cmd := exec.Command("git", "remote")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	remotes := make(map[string]string)
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		urlCmd := exec.Command("git", "remote", "get-url", name)
		urlCmd.Dir = dir
		urlOut, err := urlCmd.Output()
		if err != nil {
			continue
		}
		remotes[name] = strings.TrimSpace(string(urlOut))
	}
	return remotes
}

// readDocumentationURL reads the statichost site from task-plus.yml and returns its URL.
// For docs repos, reads directly. For project repos, reads from the -docs sibling.
func readDocumentationURL(dir string) string {
	// Try the current directory's config first
	site := readStatichostSite(dir)
	if site != "" {
		return "https://" + site + ".statichost.page/"
	}
	// Try -docs sibling
	parentDir := readParentRepo(dir)
	if parentDir == "" {
		// Not a docs repo; look for -docs sibling
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return ""
		}
		sibling := absDir + "-docs"
		site = readStatichostSite(sibling)
		if site != "" {
			return "https://" + site + ".statichost.page/"
		}
	}
	return ""
}

// readStatichostSite reads the statichost site name from task-plus.yml in dir.
func readStatichostSite(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "task-plus.yml"))
	if err != nil {
		return ""
	}
	// Simple line-based parse: look for "site:" within pages_deploy section
	lines := strings.Split(string(data), "\n")
	inDeploy := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "pages_deploy:" {
			inDeploy = true
			continue
		}
		// Exit pages_deploy section on next top-level key
		if inDeploy && len(line) > 0 && line[0] != ' ' && line[0] != '\t' && line[0] != '-' {
			break
		}
		if inDeploy && strings.HasPrefix(trimmed, "site:") {
			val := strings.TrimPrefix(trimmed, "site:")
			return strings.TrimSpace(val)
		}
	}
	return ""
}

// sortedKeys returns map keys in sorted order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
