// Package mdupdate processes auto-marker comments in markdown files.
// Markers use HTML comments: <!-- auto:name -->...<!-- /auto:name -->
package mdupdate

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/drummonds/task-plus/internal/git"
	"github.com/drummonds/task-plus/internal/readme"
)

// Options configures which markers to process.
type Options struct {
	PagesDir string // directory to scan for HTML pages (for auto:pages)
}

// pageInfo holds metadata about a page.
type pageInfo struct {
	Title    string
	Filename string
}

// pageGroup holds pages grouped by directory for sidebar navigation.
type pageGroup struct {
	Label string
	Pages []pageInfo
}

// LinkInfo holds a label/URL pair for the links section.
type LinkInfo struct {
	Label string
	URL   string
}

// Update reads a markdown file, processes auto-markers, and writes it back.
func Update(path string, opts Options) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	if opts.PagesDir == "" {
		opts.PagesDir = filepath.Dir(path)
	}

	content := string(data)
	if !strings.Contains(content, "<!-- auto:") {
		return nil
	}

	updated := UpdateContent(content, opts)
	if updated == content {
		return nil
	}

	return os.WriteFile(path, []byte(updated), 0644)
}

// UpdateContent processes auto-markers in content and returns the updated string.
func UpdateContent(content string, opts Options) string {
	if !strings.Contains(content, "<!-- auto:") {
		return content
	}

	s := content
	if updated, ok := readme.ReplaceSection(s, "toc", GenerateTOC([]byte(s))); ok {
		s = updated
	}
	if opts.PagesDir != "" {
		if updated, ok := readme.ReplaceSection(s, "pages", GeneratePagesNav(opts.PagesDir)); ok {
			s = updated
		}
	}
	if updated, ok := readme.ReplaceSection(s, "links", GenerateLinksTable()); ok {
		s = updated
	}
	return s
}

// tocHeadingRe matches markdown headings h2–h6.
var tocHeadingRe = regexp.MustCompile(`^(#{2,6})\s+(.+)$`)

// GenerateTOC returns a markdown table-of-contents from h2+ headings in content.
func GenerateTOC(content []byte) string {
	var sb strings.Builder
	sb.WriteByte('\n')
	for _, line := range strings.Split(string(content), "\n") {
		m := tocHeadingRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		level := len(m[1]) - 2 // h2=0, h3=1, etc.
		title := strings.TrimSpace(m[2])
		id := HeadingToID(title)
		indent := strings.Repeat("  ", level)
		sb.WriteString(indent + "- [" + title + "](#" + id + ")\n")
	}
	return sb.String()
}

// HeadingToID matches Goldmark's WithAutoHeadingID() algorithm:
// lowercase, keep alphanumeric, replace spaces/hyphens/underscores with '-'.
func HeadingToID(text string) string {
	text = strings.TrimSpace(text)
	var b strings.Builder
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_':
			b.WriteByte('-')
		}
	}
	return strings.ToLower(b.String())
}

// GeneratePagesNav returns a Bulma sidebar menu of all .html pages in dst,
// grouped by subdirectory.
func GeneratePagesNav(dst string) string {
	groups := scanPagesGrouped(dst)
	if len(groups) == 0 {
		return "\n"
	}
	var sb strings.Builder
	sb.WriteString("\n<aside class=\"menu\">\n")
	for _, g := range groups {
		sb.WriteString("<p class=\"menu-label\">" + g.Label + "</p>\n")
		sb.WriteString("<ul class=\"menu-list\">\n")
		for _, p := range g.Pages {
			sb.WriteString("<li><a href=\"" + p.Filename + "\">" + p.Title + "</a></li>\n")
		}
		sb.WriteString("</ul>\n")
	}
	sb.WriteString("</aside>\n")
	return sb.String()
}

// GenerateLinksTable returns a markdown table of auto-discovered links.
func GenerateLinksTable() string {
	links := discoverLinks()
	if len(links) == 0 {
		return "\n"
	}
	var sb strings.Builder
	sb.WriteString("\n| | |\n|---|---|\n")
	for _, l := range links {
		sb.WriteString("| " + l.Label + " | " + l.URL + " |\n")
	}
	return sb.String()
}

// scanPagesGrouped recursively scans dst for .html files and groups them by
// subdirectory. Top-level files go into a "Pages" group.
func scanPagesGrouped(dst string) []pageGroup {
	var rootPages []pageInfo
	subDirs := make(map[string][]pageInfo)

	_ = filepath.WalkDir(dst, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".html") || d.Name() == "index.html" {
			return nil
		}
		rel, _ := filepath.Rel(dst, path)
		rel = filepath.ToSlash(rel)
		title := extractHTMLTitle(path, d.Name())
		dir := filepath.Dir(rel)
		if dir == "." {
			rootPages = append(rootPages, pageInfo{Title: title, Filename: rel})
		} else {
			subDirs[dir] = append(subDirs[dir], pageInfo{Title: title, Filename: rel})
		}
		return nil
	})

	var groups []pageGroup
	if len(rootPages) > 0 {
		sort.Slice(rootPages, func(i, j int) bool {
			return rootPages[i].Filename < rootPages[j].Filename
		})
		groups = append(groups, pageGroup{Label: "Pages", Pages: rootPages})
	}

	var dirs []string
	for d := range subDirs {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		pages := subDirs[d]
		sort.Slice(pages, func(i, j int) bool {
			return pages[i].Filename < pages[j].Filename
		})
		groups = append(groups, pageGroup{Label: capitalize(filepath.Base(d)), Pages: pages})
	}

	return groups
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
	if idx := strings.Index(title, " - "); idx > 0 {
		title = title[:idx]
	}
	return title
}

// discoverLinks auto-discovers project links from git remotes and task-plus.yml.
// Orders: Documentation, then Sources (non-GitHub), then Mirrors (GitHub).
func discoverLinks() []LinkInfo {
	var links []LinkInfo

	cwdRemotes := gitRemoteURLs(".")

	if docURL := readDocumentationURL("."); docURL != "" {
		links = append(links, LinkInfo{Label: "Documentation", URL: docURL})
	}

	type link struct{ label, url string }
	var sources, mirrors []link
	for _, name := range sortedKeys(cwdRemotes) {
		webURL := git.URLToWeb(cwdRemotes[name])
		if webURL == "" {
			continue
		}
		switch {
		case strings.Contains(webURL, "github.com"):
			mirrors = append(mirrors, link{"Mirror (GitHub)", webURL})
		default:
			label := "Source"
			if strings.Contains(webURL, "codeberg.org") {
				label = "Source (Codeberg)"
			}
			sources = append(sources, link{label, webURL})
		}
	}
	for _, l := range sources {
		links = append(links, LinkInfo{Label: l.label, URL: l.url})
	}
	for _, l := range mirrors {
		links = append(links, LinkInfo{Label: l.label, URL: l.url})
	}

	return links
}

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

func readDocumentationURL(dir string) string {
	site := readStatichostSite(dir)
	if site != "" {
		return "https://" + site + ".statichost.page/"
	}
	return ""
}

func readStatichostSite(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "task-plus.yml"))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	inDeploy := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "pages_deploy:" {
			inDeploy = true
			continue
		}
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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
