// Package check validates task-plus.yml and Taskfile.yml configuration.
package check

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/forge"
	"github.com/drummonds/task-plus/internal/git"
	"gopkg.in/yaml.v3"
)

type level int

const (
	levelOK level = iota
	levelWarn
	levelError
)

type finding struct {
	level   level
	message string
}

func (f finding) String() string {
	switch f.level {
	case levelOK:
		return "  OK    " + f.message
	case levelWarn:
		return "  WARN  " + f.message
	case levelError:
		return "  ERROR " + f.message
	}
	return "  " + f.message
}

// Known yaml tags in Config struct.
var knownConfigKeys = map[string]bool{
	"type":              true,
	"precheck":          true,
	"check":             true,
	"changelog_format":  true,
	"wasm":              true,
	"goreleaser_config": true,
	"forge":             true,
	"release_remote":    true,
	"remotes":           true,
	"cleanup":           true,
	"fork":              true,
	"install":           true,
	"install_retries":   true,
	"languages":         true,
	"pages_build":       true,
	"pages_deploy":      true,
	"docs_repo":         true,
	"parent_repo":       true,
}

// Preferred task name → inverted (wrong) form.
var taskInversions = []struct {
	preferred string
	inverted  string
}{
	{"docs:build", "build:docs"},
	{"deps:tidy", "tidy:deps"},
	{"release:version-update", "version-update:release"},
	{"release:install", "install:release"},
	{"post:release", "release:post"},
}

// Task names that conflict with tp commands.
var taskConflicts = []string{"release", "pages"}

// Standard expected task names.
var standardTasks = []string{"fmt", "vet", "test", "check"}

// Run validates the project configuration in dir and prints a report.
// Returns an error if any ERROR-level findings exist.
func Run(dir string) error {
	var errors, warnings int

	// --- task-plus.yml ---
	fmt.Println("task-plus.yml")
	for _, f := range checkConfig(dir) {
		fmt.Println(f)
		switch f.level {
		case levelError:
			errors++
		case levelWarn:
			warnings++
		}
	}

	// --- Taskfile.yml ---
	fmt.Println("\nTaskfile.yml")
	for _, f := range checkTaskfile(dir) {
		fmt.Println(f)
		switch f.level {
		case levelError:
			errors++
		case levelWarn:
			warnings++
		}
	}

	// --- Remotes ---
	fmt.Println("\nRemotes")
	for _, f := range checkRemotes(dir) {
		fmt.Println(f)
		switch f.level {
		case levelError:
			errors++
		case levelWarn:
			warnings++
		}
	}

	// --- Cross-repo checks ---
	fmt.Println("\nCross-repo")
	for _, f := range checkCrossRepo(dir) {
		fmt.Println(f)
		switch f.level {
		case levelError:
			errors++
		case levelWarn:
			warnings++
		}
	}

	// --- Worktrees ---
	fmt.Println("\nWorktrees")
	for _, f := range checkWorktrees(dir) {
		fmt.Println(f)
		switch f.level {
		case levelError:
			errors++
		case levelWarn:
			warnings++
		}
	}

	// --- GitHub Pages ---
	fmt.Println("\nGitHub Pages")
	for _, f := range checkGitHubPages(dir) {
		fmt.Println(f)
		switch f.level {
		case levelError:
			errors++
		case levelWarn:
			warnings++
		}
	}

	// --- Statichost ---
	fmt.Println("\nStatichost")
	for _, f := range checkStatichost(dir) {
		fmt.Println(f)
		switch f.level {
		case levelError:
			errors++
		case levelWarn:
			warnings++
		}
	}

	// --- Deploy summary ---
	printDeploy(dir)

	// --- Summary line ---
	fmt.Printf("\n%d errors, %d warnings\n", errors, warnings)

	if errors > 0 {
		return fmt.Errorf("%d errors found", errors)
	}
	return nil
}

func checkConfig(dir string) []finding {
	var findings []finding

	path := filepath.Join(dir, "task-plus.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		findings = append(findings, finding{levelWarn, "File not found (using defaults)"})
		return findings
	}

	// Check YAML parses
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		findings = append(findings, finding{levelError, fmt.Sprintf("YAML parse error: %v", err)})
		return findings
	}

	// Check for unknown keys
	for key := range raw {
		if !knownConfigKeys[key] {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("Unknown field %q", key)})
		}
	}

	// Load config properly
	cfg, err := config.Load(dir)
	if err != nil {
		findings = append(findings, finding{levelError, fmt.Sprintf("Config load error: %v", err)})
		return findings
	}

	findings = append(findings, finding{levelOK, "File found and valid"})

	// Validate type
	switch cfg.Type {
	case "binary", "library", "docs":
		findings = append(findings, finding{levelOK, fmt.Sprintf("Type: %s", cfg.Type)})
	default:
		findings = append(findings, finding{levelError, fmt.Sprintf("Invalid type %q (expected: binary, library, docs)", cfg.Type)})
	}

	// Validate languages
	if len(cfg.Languages) > 0 {
		findings = append(findings, finding{levelOK, fmt.Sprintf("Languages: %s", strings.Join(cfg.Languages, ", "))})
	}
	for _, lang := range cfg.Languages {
		switch lang {
		case "go":
			if !cfg.HasGoMod() {
				findings = append(findings, finding{levelError, "Language 'go' but no go.mod found"})
			}
		case "python":
			if !cfg.HasPyproject() {
				findings = append(findings, finding{levelError, "Language 'python' but no pyproject.toml found"})
			}
		default:
			findings = append(findings, finding{levelWarn, fmt.Sprintf("Unknown language %q (expected: go, python)", lang)})
		}
	}

	// Reverse check: marker files exist but language not listed
	if cfg.HasPyproject() && !cfg.HasPython() {
		findings = append(findings, finding{levelWarn, "pyproject.toml found but 'python' not in languages"})
	}
	if cfg.HasGoMod() && !cfg.HasGo() {
		findings = append(findings, finding{levelWarn, "go.mod found but 'go' not in languages"})
	}

	// Validate changelog_format
	switch cfg.ChangelogFormat {
	case "keepachangelog", "simple":
		findings = append(findings, finding{levelOK, fmt.Sprintf("Changelog format: %s", cfg.ChangelogFormat)})
	default:
		findings = append(findings, finding{levelError, fmt.Sprintf("Invalid changelog_format %q (expected: keepachangelog, simple)", cfg.ChangelogFormat)})
	}

	// Validate forge (if explicitly set)
	if v, ok := raw["forge"]; ok {
		forgeStr, _ := v.(string)
		switch forgeStr {
		case "github", "gitlab", "forgejo":
			findings = append(findings, finding{levelOK, fmt.Sprintf("Forge: %s", forgeStr)})
		default:
			findings = append(findings, finding{levelError, fmt.Sprintf("Invalid forge %q (expected: github, gitlab, forgejo)", forgeStr)})
		}
	}

	// Check for unsupported md2html flags in pages_build
	findings = append(findings, checkMd2htmlFlags(dir, "task-plus.yml")...)

	// Validate deploy targets
	for i, t := range cfg.PagesDeploy {
		switch t.Type {
		case "github", "statichost":
			// valid
		default:
			findings = append(findings, finding{levelError, fmt.Sprintf("pages_deploy[%d]: invalid type %q (expected: github, statichost)", i, t.Type)})
			continue
		}
		if t.Type == "statichost" {
			switch t.Site {
			case "":
				findings = append(findings, finding{levelError, fmt.Sprintf("pages_deploy[%d]: statichost requires 'site' field", i)})
			case "CHANGEME":
				findings = append(findings, finding{levelWarn, fmt.Sprintf("pages_deploy[%d]: site is still 'CHANGEME'", i)})
			}
		}
	}

	return findings
}

func checkTaskfile(dir string) []finding {
	var findings []finding

	path := filepath.Join(dir, "Taskfile.yml")
	if _, err := os.Stat(path); err != nil {
		findings = append(findings, finding{levelWarn, "File not found"})
		return findings
	}
	findings = append(findings, finding{levelOK, "File found"})

	// Check standard tasks
	var present, missing []string
	for _, name := range standardTasks {
		if config.HasTaskfileTask(dir, name) {
			present = append(present, name)
		} else {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		findings = append(findings, finding{levelOK, fmt.Sprintf("Standard tasks: %s", strings.Join(present, ", "))})
	} else {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Missing standard tasks: %s", strings.Join(missing, ", "))})
	}

	// Check task name conflicts
	for _, name := range taskConflicts {
		if config.HasTaskfileTask(dir, name) {
			findings = append(findings, finding{levelError, fmt.Sprintf("Task %q conflicts with 'tp %s' command — remove it", name, name)})
		}
	}

	// Check task name inversions
	for _, inv := range taskInversions {
		hasPreferred := config.HasTaskfileTask(dir, inv.preferred)
		hasInverted := config.HasTaskfileTask(dir, inv.inverted)
		if hasInverted && !hasPreferred {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("Has '%s' — rename to '%s' (subject:action convention)", inv.inverted, inv.preferred)})
		}
	}

	// Check Go projects have a lint task using golangci-lint
	cfg, _ := config.Load(dir)
	if cfg != nil && cfg.HasGo() {
		if config.HasTaskfileTask(dir, "lint") {
			findings = append(findings, finding{levelOK, "lint task found (golangci-lint)"})
		} else {
			findings = append(findings, finding{levelWarn, "Go project missing 'lint' task — add golangci-lint (https://golangci-lint.run/)"})
		}
	}

	// Check for unsupported md2html flags
	findings = append(findings, checkMd2htmlFlags(dir, "Taskfile.yml")...)

	// Check .gitignore includes .task/ (taskfile checksum cache)
	if gitignoreContains(dir, ".task") {
		findings = append(findings, finding{levelOK, ".task/ in .gitignore"})
	} else {
		findings = append(findings, finding{levelWarn, ".task/ not in .gitignore — add '.task' to avoid committing checksum cache"})
	}

	return findings
}

func checkRemotes(dir string) []finding {
	var findings []finding

	// Check git has at least one remote
	remotes, err := git.Remotes(dir)
	if err != nil {
		findings = append(findings, finding{levelError, fmt.Sprintf("Cannot list git remotes: %v", err)})
		return findings
	}
	if len(remotes) == 0 {
		findings = append(findings, finding{levelError, "No git remotes configured — release requires at least one remote"})
		return findings
	}

	cfg, err := config.Load(dir)
	if err != nil {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot load config: %v", err)})
		return findings
	}

	// Check each configured push-target remote exists in git
	for _, name := range cfg.Remotes {
		url, err := git.RemoteURL(dir, name)
		if err != nil {
			findings = append(findings, finding{levelError, fmt.Sprintf("Remote %q in config but not in git", name)})
			continue
		}
		forgeType := forge.DetectFromURL(url)
		hasCLI := forge.Forge{Type: forgeType}.HasCLI()
		extra := ""
		if hasCLI {
			extra = ", cli: yes"
		}
		findings = append(findings, finding{levelOK, fmt.Sprintf("%-16s %s (%s%s)", name, url, forgeType, extra)})
	}

	return findings
}

func checkCrossRepo(dir string) []finding {
	var findings []finding

	cfg, err := config.Load(dir)
	if err != nil {
		return findings
	}

	if cfg.IsDocs() {
		// Running from a -docs repo — suggest combining
		findings = append(findings, finding{levelWarn, "Separate -docs repo detected — consider running 'tp pages combine' from the main project to consolidate"})
		parentDir := cfg.ResolveParentRepo()
		if parentDir != "" {
			findings = append(findings, finding{levelOK, fmt.Sprintf("Parent repo: %s", parentDir)})
		}
	} else {
		// Running from main repo — check for leftover -docs sibling
		docsDir := cfg.ResolveDocsRepo()
		if docsDir == "" {
			findings = append(findings, finding{levelOK, "No -docs sibling (integrated docs)"})
		} else {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("Separate -docs repo found: %s — run 'tp pages combine' to consolidate", docsDir)})
		}
	}

	return findings
}

func checkWorktrees(dir string) []finding {
	var findings []finding

	out, err := git.Run(dir, "worktree", "list", "--porcelain")
	if err != nil {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot list worktrees: %v", err)})
		return findings
	}

	// Parse porcelain output: blocks separated by blank lines.
	// Each block has "worktree <path>" and "branch refs/heads/<name>".
	type wt struct {
		path, branch string
		bare         bool
	}
	var worktrees []wt
	var cur wt
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if cur.path != "" {
				worktrees = append(worktrees, cur)
			}
			cur = wt{path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			cur.branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "bare":
			cur.bare = true
		}
	}
	if cur.path != "" {
		worktrees = append(worktrees, cur)
	}

	if len(worktrees) <= 1 {
		findings = append(findings, finding{levelOK, "No extra worktrees"})
		return findings
	}

	findings = append(findings, finding{levelOK, fmt.Sprintf("%d worktrees:", len(worktrees))})
	for _, w := range worktrees {
		label := w.branch
		if label == "" {
			label = "(detached)"
		}
		if w.bare {
			label = "(bare)"
		}
		findings = append(findings, finding{levelOK, fmt.Sprintf("  %-40s %s", w.path, label)})
	}

	return findings
}

func checkGitHubPages(dir string) []finding {
	var findings []finding

	// Find the GitHub remote
	cfg, err := config.Load(dir)
	if err != nil {
		findings = append(findings, finding{levelWarn, "Cannot load config"})
		return findings
	}

	var ghRemote string
	for _, name := range cfg.Remotes {
		url, err := git.RemoteURL(dir, name)
		if err != nil {
			continue
		}
		if strings.Contains(url, "github.com") {
			ghRemote = name
			break
		}
	}
	if ghRemote == "" {
		findings = append(findings, finding{levelOK, "No GitHub remote configured"})
		return findings
	}

	// Check if gh-pages branch exists (locally or remote)
	_, errLocal := git.Run(dir, "rev-parse", "--verify", "gh-pages")
	_, errRemote := git.Run(dir, "rev-parse", "--verify", "remotes/"+ghRemote+"/gh-pages")

	if errLocal != nil && errRemote != nil {
		findings = append(findings, finding{levelOK, "No gh-pages branch (GitHub Pages absent)"})
		return findings
	}

	// gh-pages exists — check what's in it
	ref := "gh-pages"
	if errLocal != nil {
		ref = ghRemote + "/gh-pages"
	}

	// List files in gh-pages root
	files, err := git.Run(dir, "ls-tree", "--name-only", ref)
	if err != nil {
		findings = append(findings, finding{levelWarn, "gh-pages branch exists but cannot list contents"})
		return findings
	}

	fileList := strings.Split(strings.TrimSpace(files), "\n")

	// Check if it's a simple redirect: just index.html (and maybe CNAME/.nojekyll)
	htmlFiles := 0
	hasIndex := false
	for _, f := range fileList {
		if strings.HasSuffix(f, ".html") {
			htmlFiles++
		}
		if f == "index.html" {
			hasIndex = true
		}
	}

	if !hasIndex {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("gh-pages branch has %d files but no index.html", len(fileList))})
		return findings
	}

	if htmlFiles == 1 && hasIndex {
		// Likely a redirect page — check content
		content, err := git.Run(dir, "show", ref+":index.html")
		if err == nil && isRedirectPage(content) {
			target := extractRedirectTarget(content)
			findings = append(findings, finding{levelOK, fmt.Sprintf("Simple redirect to %s", target)})
			return findings
		}
	}

	findings = append(findings, finding{levelOK, fmt.Sprintf("gh-pages branch with %d files (%d HTML)", len(fileList), htmlFiles)})
	return findings
}

// isRedirectPage checks if HTML content is a simple redirect/refresh page.
func isRedirectPage(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "http-equiv") && strings.Contains(lower, "refresh")
}

// extractRedirectTarget extracts the URL from a meta refresh tag.
func extractRedirectTarget(content string) string {
	lower := strings.ToLower(content)
	idx := strings.Index(lower, "url=")
	if idx < 0 {
		return "(unknown)"
	}
	rest := content[idx+4:]
	// Strip quotes
	rest = strings.TrimLeft(rest, "'\"")
	// Find end
	end := strings.IndexAny(rest, "'\" >")
	if end > 0 {
		rest = rest[:end]
	}
	return rest
}

// unsupportedMd2htmlFlags lists flags that don't exist in md2html.
var unsupportedMd2htmlFlags = []string{"--index"}

// checkMd2htmlFlags scans a file for md2html invocations with unsupported flags.
func checkMd2htmlFlags(dir, filename string) []finding {
	data, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil {
		return nil
	}
	var findings []finding
	for i, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "md2html") {
			continue
		}
		for _, flag := range unsupportedMd2htmlFlags {
			if strings.Contains(line, flag) {
				findings = append(findings, finding{levelError, fmt.Sprintf("%s:%d: unsupported md2html flag %q", filename, i+1, flag)})
			}
		}
	}
	return findings
}

// gitignoreContains returns true if .gitignore contains a line matching the pattern.
func gitignoreContains(dir, pattern string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// Match ".task", ".task/", ".task/*" etc.
		if line == pattern || line == pattern+"/" {
			return true
		}
	}
	return false
}

// statichostHTTPClient is the HTTP client used for statichost reachability checks.
// Overridable in tests.
var statichostHTTPClient = &http.Client{Timeout: 5 * time.Second}

func checkStatichost(dir string) []finding {
	var findings []finding

	cfg, err := config.Load(dir)
	if err != nil {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot load config: %v", err)})
		return findings
	}

	var sites []string
	for _, t := range cfg.PagesDeploy {
		if t.Type != "statichost" {
			continue
		}
		if t.Site != "" {
			sites = append(sites, t.Site)
		}
		if t.HasRCSite() {
			sites = append(sites, t.RCSite)
		}
	}

	if len(sites) == 0 {
		findings = append(findings, finding{levelOK, "No statichost targets configured"})
		return findings
	}

	for _, site := range sites {
		url := "https://" + site + ".statichost.page/"
		resp, err := statichostHTTPClient.Head(url)
		if err != nil {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("%s unreachable: %v", site, err)})
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			findings = append(findings, finding{levelOK, fmt.Sprintf("%s reachable (%d)", site, resp.StatusCode)})
		} else {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("%s returned HTTP %d", site, resp.StatusCode)})
		}
	}

	return findings
}

func printDeploy(dir string) {
	cfg, err := config.Load(dir)
	if err != nil || len(cfg.PagesDeploy) == 0 {
		return
	}

	fmt.Println("\nDeploy targets:")
	for _, t := range cfg.PagesDeploy {
		dirLabel := ""
		if t.Dir != "" {
			dirLabel = fmt.Sprintf(", dir: %s", t.Dir)
		}
		switch t.Type {
		case "statichost":
			fmt.Printf("  %-16s site: %s (statichost.eu%s)\n", t.Type, t.Site, dirLabel)
		case "github":
			fmt.Printf("  %-16s GitHub Pages (gh-pages branch%s)\n", t.Type, dirLabel)
		default:
			fmt.Printf("  %-16s %s\n", t.Type, t.Site)
		}
	}
}
