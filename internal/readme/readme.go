// Package readme provides marker-based auto-updating of README.md sections.
// Markers use HTML comments: <!-- auto:name -->...<!-- /auto:name -->
package readme

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/git"
)

// Update reads README.md in dir, replaces auto-marker sections, and writes back.
// If version is non-empty, updates the version marker. Always updates the links marker.
// Returns nil if README.md has no markers.
func Update(dir, version string) error {
	path := filepath.Join(dir, "README.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading README.md: %w", err)
	}
	content := string(data)

	if !strings.Contains(content, "<!-- auto:") {
		return nil // no markers, nothing to do
	}

	changed := false

	// Update version marker
	if version != "" {
		if replacement := GenerateVersion(version); replacement != "" {
			if updated, ok := ReplaceSection(content, "version", replacement); ok {
				content = updated
				changed = true
			}
		}
	}

	// Update links marker
	linksTable := GenerateLinksTable(dir)
	if linksTable != "" {
		if updated, ok := ReplaceSection(content, "links", linksTable); ok {
			content = updated
			changed = true
		}
	}

	if !changed {
		return nil
	}

	return os.WriteFile(path, []byte(content), 0644)
}

// ReplaceSection finds <!-- auto:name --> and <!-- /auto:name --> markers and
// replaces everything between them. Returns the updated string and true if
// the markers were found.
func ReplaceSection(content, name, replacement string) (string, bool) {
	open := "<!-- auto:" + name + " -->"
	close := "<!-- /auto:" + name + " -->"

	start := strings.Index(content, open)
	if start < 0 {
		return content, false
	}
	end := strings.Index(content[start:], close)
	if end < 0 {
		return content, false
	}
	end += start // adjust to absolute position

	result := content[:start+len(open)] + replacement + content[end:]
	return result, true
}

// GenerateVersion returns the formatted version string for the marker.
func GenerateVersion(version string) string {
	return "Latest: " + version
}

// GenerateLinksTable builds a markdown links table from git remotes and docs sibling config.
func GenerateLinksTable(dir string) string {
	var rows []struct{ label, url string }

	cfg, err := config.Load(dir)
	if err != nil {
		cfg = &config.Config{Dir: dir}
	}

	// Documentation URL from statichost config (local or -docs sibling)
	if docURL := docsURL(cfg); docURL != "" {
		rows = append(rows, struct{ label, url string }{"Documentation", docURL})
	}

	// PyPI link for Python projects
	if cfg.HasPython() {
		if name := cfg.PypiPackageName(); name != "" {
			rows = append(rows, struct{ label, url string }{"PyPI", "https://pypi.org/project/" + name + "/"})
		}
	}

	// Source links from git remotes (Source before Mirror)
	type link struct{ label, url string }
	var sources, mirrors []link
	remotes, _ := git.Remotes(dir)
	for _, name := range remotes {
		url, err := git.RemoteURL(dir, name)
		if err != nil {
			continue
		}
		webURL := git.URLToWeb(url)
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
		rows = append(rows, struct{ label, url string }{l.label, l.url})
	}
	for _, l := range mirrors {
		rows = append(rows, struct{ label, url string }{l.label, l.url})
	}

	// Docs repo link from sibling
	if docsDir := cfg.ResolveDocsRepo(); docsDir != "" {
		docsRemotes, _ := git.Remotes(docsDir)
		for _, name := range docsRemotes {
			url, err := git.RemoteURL(docsDir, name)
			if err != nil {
				continue
			}
			webURL := git.URLToWeb(url)
			if webURL != "" {
				rows = append(rows, struct{ label, url string }{"Docs repo", webURL})
				break // only need one
			}
		}
	}

	if len(rows) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n| | |\n|---|---|\n")
	for _, r := range rows {
		sb.WriteString("| " + r.label + " | " + r.url + " |\n")
	}
	return sb.String()
}

// docsURL returns the documentation URL from statichost config.
// Checks local config first (combined docs), then -docs sibling.
func docsURL(cfg *config.Config) string {
	// Check local config first
	for _, target := range cfg.PagesDeploy {
		if target.Type == "statichost" && target.Site != "" {
			return "https://" + target.Site + ".statichost.page/"
		}
	}
	// Fall back to -docs sibling
	docsDir := cfg.ResolveDocsRepo()
	if docsDir == "" {
		return ""
	}
	docsCfg, err := config.Load(docsDir)
	if err != nil {
		return ""
	}
	for _, target := range docsCfg.PagesDeploy {
		if target.Type == "statichost" && target.Site != "" {
			return "https://" + target.Site + ".statichost.page/"
		}
	}
	return ""
}
