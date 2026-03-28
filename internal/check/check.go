// Package check validates task-plus.yml and Taskfile.yml configuration.
package check

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"codeberg.org/hum3/task-plus/internal/changelog"
	"codeberg.org/hum3/task-plus/internal/config"
	"codeberg.org/hum3/task-plus/internal/favicon"
	"codeberg.org/hum3/task-plus/internal/forge"
	"codeberg.org/hum3/task-plus/internal/git"
	"codeberg.org/hum3/task-plus/internal/version"
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

// ANSI escape for green tick and reset.
const (
	greenTick = "\033[32m\u2714\033[0m"
	redCross  = "\033[31m\u2718\033[0m"
)

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
	"linter":            true,
	"pages_build":       true,
	"pages_deploy":      true,
	"retract_reviewed":  true,
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

// Standard expected task names (language-agnostic).
var standardTasks = []string{"test", "check"}

// Go-specific standard tasks.
var goTasks = []string{"fmt", "vet"}

// section groups a named set of findings for display.
type section struct {
	name     string
	findings []finding
}

// printSection outputs a section in verbose or compact mode.
// Returns the number of errors and warnings in the section.
func printSection(s section, verbose bool) (errors, warnings int) {
	for _, f := range s.findings {
		switch f.level {
		case levelError:
			errors++
		case levelWarn:
			warnings++
		}
	}

	if verbose {
		fmt.Println(s.name)
		for _, f := range s.findings {
			fmt.Println(f)
		}
		return
	}

	// Compact mode: green tick if all OK, otherwise show heading + warnings/errors only.
	if errors == 0 && warnings == 0 {
		fmt.Printf("%s %s\n", greenTick, s.name)
		return
	}

	icon := redCross
	if errors == 0 {
		icon = "!"
	}
	fmt.Printf("%s %s\n", icon, s.name)
	for _, f := range s.findings {
		if f.level != levelOK {
			fmt.Println(f)
		}
	}
	return
}

// Run validates the project configuration in dir and prints a report.
// Returns an error if any ERROR-level findings exist.
func Run(dir string, verbose bool) error {
	// Phase 1: Local checks (fast, no network)
	localSections := []section{
		{"task-plus.yml", checkConfig(dir)},
		{"Taskfile.yml", checkTaskfile(dir)},
		{"Go module", checkGoModule(dir)},
		{"Remotes", checkRemotes(dir)},
		{"Cross-repo", checkCrossRepo(dir)},
		{"Worktrees", checkWorktrees(dir)},
		{"GitHub Pages", checkGitHubPages(dir)},
		{"Favicon", checkFavicon(dir)},
	}

	var totalErrors, totalWarnings int
	for i, s := range localSections {
		if verbose && i > 0 {
			fmt.Println()
		}
		e, w := printSection(s, verbose)
		totalErrors += e
		totalWarnings += w
	}

	// Phase 2: Remote checks (network required, may be slow)
	fmt.Println("\nChecking external resources...")
	remoteSections := []section{
		checkVersionSection(dir),
		{"Go proxy", checkGoProxy(dir)},
		{"Statichost", checkStatichost(dir)},
	}

	for _, s := range remoteSections {
		if verbose {
			fmt.Println()
		}
		e, w := printSection(s, verbose)
		totalErrors += e
		totalWarnings += w
	}

	// Deploy summary (verbose only)
	if verbose {
		printDeploy(dir)
	}

	// Summary line
	fmt.Printf("\n%d errors, %d warnings\n", totalErrors, totalWarnings)

	if totalErrors > 0 {
		return fmt.Errorf("%d errors found", totalErrors)
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

	// Validate linter override (if explicitly set)
	if cfg.Linter != "" {
		switch cfg.Linter {
		case "staticcheck", "golangci-lint":
			findings = append(findings, finding{levelOK, fmt.Sprintf("Linter: %s", cfg.Linter)})
		default:
			findings = append(findings, finding{levelError, fmt.Sprintf("Invalid linter %q (expected: staticcheck, golangci-lint)", cfg.Linter)})
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

	// Build the task checklist: language-agnostic + language-specific
	cfg, _ := config.Load(dir)
	tasks := standardTasks
	if cfg != nil && cfg.HasGo() {
		tasks = append(goTasks, tasks...)
	}

	// Check standard tasks
	var present, missing []string
	for _, name := range tasks {
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

	// Check Go projects have a lint task and detect linter tool
	if cfg != nil && cfg.HasGo() {
		taskfileData, _ := os.ReadFile(filepath.Join(dir, "Taskfile.yml"))
		if config.HasTaskfileTask(dir, "lint") {
			switch {
			case taskfileData != nil && taskContains(taskfileData, "lint", "golangci-lint"):
				findings = append(findings, finding{levelOK, "lint task uses golangci-lint"})
			case taskfileData != nil && taskContains(taskfileData, "lint", "staticcheck"):
				if cfg.Linter == "staticcheck" {
					findings = append(findings, finding{levelOK, "lint task uses staticcheck (override)"})
				} else {
					findings = append(findings, finding{levelWarn, "lint uses staticcheck — migrate to golangci-lint (wraps staticcheck + more; https://golangci-lint.run/)"})
				}
			default:
				findings = append(findings, finding{levelOK, "lint task found"})
			}
		} else {
			findings = append(findings, finding{levelWarn, "Go project missing 'lint' task — add golangci-lint (https://golangci-lint.run/)"})
		}

		// Check Go 1.26+ projects include 'go fix' in fmt task
		major, minor, ok := readGoVersion(dir)
		if ok && (major > 1 || (major == 1 && minor >= 26)) {
			if taskfileData != nil && taskContains(taskfileData, "fmt", "go fix") {
				findings = append(findings, finding{levelOK, "fmt task includes go fix"})
			} else {
				findings = append(findings, finding{levelWarn, "Go 1.26+ project — add 'go fix ./...' to fmt task"})
			}
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

func checkGoModule(dir string) []finding {
	var findings []finding

	cfg, err := config.Load(dir)
	if err != nil {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot load config: %v", err)})
		return findings
	}

	if !cfg.HasGoMod() {
		findings = append(findings, finding{levelOK, "Not a Go project"})
		return findings
	}

	// Read module path from go.mod
	modPath, err := readGoModulePath(filepath.Join(dir, "go.mod"))
	if err != nil {
		findings = append(findings, finding{levelError, fmt.Sprintf("Cannot read go.mod: %v", err)})
		return findings
	}

	// Get primary remote URL
	remoteName := cfg.PrimaryRemote()
	remoteURL, err := git.RemoteURL(dir, remoteName)
	if err != nil {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot get URL for remote %q: %v", remoteName, err)})
		return findings
	}

	// Normalise remote URL to module path form
	expected := remoteURLToModulePath(remoteURL)
	if expected == "" {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot parse remote URL %q", remoteURL)})
		return findings
	}

	if modPath == expected {
		findings = append(findings, finding{levelOK, fmt.Sprintf("module %s matches remote %q", modPath, remoteName)})
	} else {
		findings = append(findings, finding{levelError, fmt.Sprintf("module %q does not match remote %q (%s)", modPath, remoteName, expected)})
	}

	return findings
}

// readGoModulePath extracts the module path from a go.mod file.
func readGoModulePath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(line[len("module "):]), nil
		}
	}
	return "", fmt.Errorf("no module directive found")
}

// remoteURLToModulePath normalises a git remote URL to a Go module path.
// Examples:
//
//	ssh://git@codeberg.org/hum3/task-plus.git → codeberg.org/hum3/task-plus
//	git@github.com:drummonds/task-plus.git    → github.com/drummonds/task-plus
//	https://github.com/drummonds/task-plus    → github.com/drummonds/task-plus
func remoteURLToModulePath(url string) string {
	s := url

	// Strip scheme
	for _, prefix := range []string{"ssh://", "https://", "http://"} {
		if strings.HasPrefix(s, prefix) {
			s = s[len(prefix):]
			break
		}
	}

	// Strip user@ prefix
	if at := strings.Index(s, "@"); at >= 0 {
		s = s[at+1:]
	}

	// Convert SCP-style colon to slash (git@github.com:org/repo)
	if colon := strings.Index(s, ":"); colon >= 0 {
		// Only convert if there's no slash before the colon (not a port)
		if !strings.Contains(s[:colon], "/") {
			s = s[:colon] + "/" + s[colon+1:]
		}
	}

	// Strip .git suffix
	s = strings.TrimSuffix(s, ".git")

	// Strip trailing slash
	s = strings.TrimRight(s, "/")

	return s
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

	findings = append(findings, finding{levelWarn, fmt.Sprintf("%d worktrees:", len(worktrees))})
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
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			findings = append(findings, finding{levelOK, fmt.Sprintf("%s reachable (%d)", site, resp.StatusCode)})
		} else {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("%s returned HTTP %d", site, resp.StatusCode)})
		}
	}

	return findings
}

func checkGoProxy(dir string) []finding {
	var findings []finding

	cfg, err := config.Load(dir)
	if err != nil || !cfg.HasGoMod() {
		findings = append(findings, finding{levelOK, "Not a Go project"})
		return findings
	}

	modPath, err := readGoModulePath(filepath.Join(dir, "go.mod"))
	if err != nil {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot read go.mod: %v", err)})
		return findings
	}

	tags, err := git.Tags(dir)
	if err != nil {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot read tags: %v", err)})
		return findings
	}
	latest, ok := version.LatestFromTags(tags)
	if !ok {
		findings = append(findings, finding{levelOK, "No version tags"})
		return findings
	}

	tagName := latest.String() // "vX.Y.Z"
	url := fmt.Sprintf("https://proxy.golang.org/%s/@v/%s.info", modPath, tagName)
	resp, err := statichostHTTPClient.Get(url)
	if err != nil {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("proxy unreachable: %v", err)})
		return findings
	}
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		findings = append(findings, finding{levelOK, fmt.Sprintf("%s@%s indexed", modPath, tagName)})
	} else {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("%s@%s not on proxy (HTTP %d)", modPath, tagName, resp.StatusCode)})
	}

	return findings
}

func checkFavicon(dir string) []finding {
	var findings []finding

	cfg, err := config.Load(dir)
	if err != nil {
		return findings
	}

	if !cfg.HasPagesDeploy() {
		findings = append(findings, finding{levelOK, "No deploy targets (favicon not needed)"})
		return findings
	}

	// Check each deploy target's docs directory for a favicon
	seen := make(map[string]bool)
	for _, t := range cfg.PagesDeploy {
		docsDir := filepath.Join(dir, t.DocsDir())
		if seen[docsDir] {
			continue
		}
		seen[docsDir] = true

		if favicon.Exists(docsDir) {
			findings = append(findings, finding{levelOK, fmt.Sprintf("Found in %s/", t.DocsDir())})
		} else {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("No favicon in %s/ — run 'tp favicon' to generate one", t.DocsDir())})
		}
	}

	return findings
}

// checkVersionSection checks version consistency across changelog, local tags,
// remote tags, and pyproject.toml. Returns a section with the version in the
// name when all sources agree.
func checkVersionSection(dir string) section {
	var findings []finding

	cfg, _ := config.Load(dir)

	// 1. Local tags
	tags, err := git.Tags(dir)
	if err != nil {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot read git tags: %v", err)})
		return section{"Version", findings}
	}
	latest, hasTag := version.LatestFromTags(tags)

	// 2. Changelog
	clVer := changelog.LatestVersion(dir)

	// 3. pyproject.toml (Python only)
	var pyVer string
	if cfg != nil && cfg.HasPython() {
		pyVer = cfg.ReadPyprojectVersion()
	}

	// If no version sources exist at all, that's fine
	if !hasTag && clVer == "" && pyVer == "" {
		findings = append(findings, finding{levelOK, "No version tags or changelog entries found"})
		return section{"Version", findings}
	}

	// Determine the canonical version (latest tag wins, fall back to changelog)
	var canonical string
	if hasTag {
		canonical = latest.TagString()
	} else if clVer != "" {
		canonical = clVer
	} else {
		canonical = pyVer
	}

	// Compare each source against canonical
	allMatch := true

	if hasTag {
		findings = append(findings, finding{levelOK, fmt.Sprintf("local tag: v%s", latest.TagString())})
	} else {
		findings = append(findings, finding{levelWarn, fmt.Sprintf("no local tag for %s", canonical)})
		allMatch = false
	}

	if clVer != "" {
		if clVer == canonical {
			findings = append(findings, finding{levelOK, fmt.Sprintf("changelog: %s", clVer)})
		} else {
			findings = append(findings, finding{levelError, fmt.Sprintf("changelog (%s) != latest tag (v%s)", clVer, canonical)})
			allMatch = false
		}
	} else {
		if _, err := os.Stat(filepath.Join(dir, "CHANGELOG.md")); err == nil {
			findings = append(findings, finding{levelWarn, "CHANGELOG.md has no version entries"})
			allMatch = false
		}
	}

	if pyVer != "" {
		if pyVer == canonical {
			findings = append(findings, finding{levelOK, fmt.Sprintf("pyproject.toml: %s", pyVer)})
		} else {
			findings = append(findings, finding{levelError, fmt.Sprintf("pyproject.toml (%s) != latest tag (v%s)", pyVer, canonical)})
			allMatch = false
		}
	}

	// 4. Remote tags
	if cfg != nil && hasTag {
		tagName := latest.String() // "vX.Y.Z"
		for _, remote := range cfg.Remotes {
			if !git.HasRemote(dir, remote) {
				continue
			}
			exists, err := git.RemoteTagExists(dir, remote, tagName)
			if err != nil {
				findings = append(findings, finding{levelWarn, fmt.Sprintf("%s: cannot check remote tags: %v", remote, err)})
				allMatch = false
				continue
			}
			if exists {
				findings = append(findings, finding{levelOK, fmt.Sprintf("%s: %s", remote, tagName)})
			} else {
				findings = append(findings, finding{levelWarn, fmt.Sprintf("%s: missing %s", remote, tagName)})
				allMatch = false
			}
		}
	}

	// 5. Retract review check (Go projects with multiple remotes)
	if cfg != nil && cfg.HasGoMod() && len(cfg.Remotes) > 1 && hasTag {
		reviewed := cfg.RetractReviewed
		if reviewed == "" {
			findings = append(findings, finding{levelWarn, "retract_reviewed not set in task-plus.yml (multi-remote Go project)"})
			allMatch = false
		} else {
			rv, err := version.Parse(reviewed)
			if err != nil {
				findings = append(findings, finding{levelWarn, fmt.Sprintf("retract_reviewed: invalid version %q", reviewed)})
				allMatch = false
			} else if rv.Less(latest) {
				findings = append(findings, finding{levelWarn, fmt.Sprintf("retract_reviewed (%s) behind latest tag (v%s)", reviewed, latest.TagString())})
				allMatch = false
			} else {
				findings = append(findings, finding{levelOK, fmt.Sprintf("retract_reviewed: %s", reviewed)})
			}
		}
	}

	name := "Version"
	if allMatch && canonical != "" {
		name = fmt.Sprintf("Version (v%s)", canonical)
	}
	return section{name, findings}
}

// taskContains reports whether the named task's block in Taskfile data
// contains substr anywhere in its indented lines.
func taskContains(data []byte, taskName string, substr string) bool {
	prefix := "  " + taskName + ":"
	lines := strings.Split(string(data), "\n")
	inTasks := false
	inTarget := false
	for _, line := range lines {
		if line == "tasks:" {
			inTasks = true
			continue
		}
		if inTasks && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inTasks = false
			inTarget = false
		}
		if !inTasks {
			continue
		}
		if !inTarget {
			if len(line) >= len(prefix) && line[:len(prefix)] == prefix {
				if len(line) == len(prefix) || line[len(prefix)] == ' ' {
					inTarget = true
				}
			}
			continue
		}
		// A line at exactly 2-space indent starts a new task
		if len(line) >= 3 && line[0] == ' ' && line[1] == ' ' && line[2] != ' ' && line[2] != '\t' {
			return false
		}
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}

// readGoVersion extracts the major.minor Go version from go.mod in dir.
func readGoVersion(dir string) (major, minor int, ok bool) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "go ") {
			continue
		}
		ver := strings.TrimPrefix(line, "go ")
		parts := strings.SplitN(ver, ".", 3)
		if len(parts) < 2 {
			return 0, 0, false
		}
		m, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, false
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
		return m, n, true
	}
	return 0, 0, false
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
