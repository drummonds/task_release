// Package readme provides marker-based auto-updating of README.md sections.
// Markers use HTML comments: <!-- auto:name -->...<!-- /auto:name -->
package readme

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/hum3/task-plus/internal/config"
	"codeberg.org/hum3/task-plus/internal/deploy"
	"codeberg.org/hum3/task-plus/internal/git"
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

	// Documentation URLs from statichost config (local or -docs sibling)
	for _, dl := range docsLinks(cfg) {
		rows = append(rows, struct{ label, url string }{dl.label, dl.url})
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

type docLink struct{ label, url string }

// docsLinks returns documentation URLs for all statichost deploy targets.
// The first target is labelled "Documentation"; subsequent targets derive
// their label from the site name. RC sites get "RC Documentation" / "RC <label>".
// Checks local config first, falls back to -docs sibling.
func docsLinks(cfg *config.Config) []docLink {
	targets := statichostTargets(cfg)
	if len(targets) == 0 {
		// Fall back to -docs sibling
		docsDir := cfg.ResolveDocsRepo()
		if docsDir == "" {
			return nil
		}
		docsCfg, err := config.Load(docsDir)
		if err != nil {
			return nil
		}
		targets = statichostTargets(docsCfg)
	}
	if len(targets) == 0 {
		return nil
	}

	var links []docLink
	for i, t := range targets {
		label := "Documentation"
		if i > 0 {
			label = siteLabel(t.Site)
		}
		links = append(links, docLink{label, "https://" + t.Site + ".statichost.page/"})
		if t.HasRCSite() {
			rcLabel := "RC " + label
			links = append(links, docLink{rcLabel, "https://" + t.RCSite + ".statichost.page/"})
		}
	}
	return links
}

// statichostTargets returns all statichost deploy targets with a site configured.
func statichostTargets(cfg *config.Config) []deploy.Target {
	var out []deploy.Target
	for _, t := range cfg.PagesDeploy {
		if t.Type == "statichost" && t.Site != "" {
			out = append(out, t)
		}
	}
	return out
}

// siteLabel derives a human-readable label from a statichost site name.
// e.g. "blog-bytestone" → "Blog Bytestone", "h3-docs" → "Docs".
func siteLabel(site string) string {
	// Strip common h3- prefix
	s := strings.TrimPrefix(site, "h3-")
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
