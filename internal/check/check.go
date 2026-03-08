// Package check validates task-plus.yml and Taskfile.yml configuration.
package check

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	"remotes":           true,
	"cleanup":           true,
	"fork":              true,
	"install":           true,
	"install_retries":   true,
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
			if t.Site == "" {
				findings = append(findings, finding{levelError, fmt.Sprintf("pages_deploy[%d]: statichost requires 'site' field", i)})
			} else if t.Site == "CHANGEME" {
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
		// Running from a -docs repo — check parent
		parentDir := cfg.ResolveParentRepo()
		if parentDir == "" {
			findings = append(findings, finding{levelWarn, "No parent repo found (expected sibling without -docs suffix)"})
			return findings
		}
		findings = append(findings, finding{levelOK, fmt.Sprintf("Parent repo: %s", parentDir)})

		parentCfg, err := config.Load(parentDir)
		if err != nil {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot load parent config: %v", err)})
			return findings
		}
		if config.HasDocsDir(parentDir) {
			findings = append(findings, finding{levelWarn, "Parent repo still has docs/ — run 'tp pages migrate clean' from parent"})
		}
		if parentCfg.HasPagesDeploy() {
			findings = append(findings, finding{levelWarn, "Parent repo still has pages_deploy — should be in -docs repo only"})
		}
		if parentCfg.HasPagesBuild() {
			findings = append(findings, finding{levelWarn, "Parent repo still has pages_build — should be in -docs repo only"})
		}

		// Check for .md files without DOC- prefix in -docs repo
		findings = append(findings, checkDocsMdPrefix(dir)...)
	} else {
		// Running from main repo — check for -docs sibling
		docsDir := cfg.ResolveDocsRepo()
		if docsDir == "" {
			findings = append(findings, finding{levelOK, "No -docs sibling (integrated docs)"})
			return findings
		}
		findings = append(findings, finding{levelOK, fmt.Sprintf("Docs repo: %s", docsDir)})

		docsCfg, err := config.Load(docsDir)
		if err != nil {
			findings = append(findings, finding{levelError, fmt.Sprintf("Cannot load -docs config: %v", err)})
			return findings
		}
		if docsCfg.Type != "docs" {
			findings = append(findings, finding{levelError, fmt.Sprintf("-docs repo has type %q — expected 'docs'", docsCfg.Type)})
		}
		if config.HasDocsDir(dir) {
			findings = append(findings, finding{levelWarn, "Main repo still has docs/ — run 'tp pages migrate clean'"})
		}
		if cfg.HasPagesDeploy() {
			findings = append(findings, finding{levelWarn, "Main repo has pages_deploy — should be in -docs repo only"})
		}
		if cfg.HasPagesBuild() {
			findings = append(findings, finding{levelWarn, "Main repo has pages_build — should be in -docs repo only"})
		}

		// Check docs repo has at least one git remote
		docsRemotes, err := git.Remotes(docsDir)
		if err != nil {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("Cannot list -docs remotes: %v", err)})
		} else if len(docsRemotes) == 0 {
			findings = append(findings, finding{levelError, "Docs repo has no git remotes — release will fail to push docs"})
		} else if !git.HasRemote(docsDir, "origin") {
			findings = append(findings, finding{levelWarn, "Docs repo has no 'origin' remote"})
		} else {
			findings = append(findings, finding{levelOK, "Docs repo has origin remote"})
		}
	}

	return findings
}

// checkDocsMdPrefix warns about .md files in a -docs repo root that lack the DOC- prefix.
// Files like go.md, CHANGELOG.md etc. belong in the parent project; docs-repo markdown
// should use DOC-README.md, DOC-CHANGELOG.md etc. to avoid confusion.
func checkDocsMdPrefix(dir string) []finding {
	var findings []finding
	entries, err := os.ReadDir(dir)
	if err != nil {
		return findings
	}

	// Allowlisted names that don't need the DOC- prefix.
	allowed := map[string]bool{
		"LICENSE.md": true,
	}

	var bad []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		if strings.HasPrefix(name, "DOC-") {
			continue
		}
		if allowed[name] {
			continue
		}
		bad = append(bad, name)
	}
	if len(bad) == 0 {
		findings = append(findings, finding{levelOK, "All .md files use DOC- prefix"})
	} else {
		for _, name := range bad {
			findings = append(findings, finding{levelWarn, fmt.Sprintf("%s should be renamed to DOC-%s (docs-repo convention)", name, name)})
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
		switch t.Type {
		case "statichost":
			fmt.Printf("  %-16s site: %s (statichost.eu)\n", t.Type, t.Site)
		case "github":
			fmt.Printf("  %-16s GitHub Pages (gh-pages branch)\n", t.Type)
		default:
			fmt.Printf("  %-16s %s\n", t.Type, t.Site)
		}
	}
}
