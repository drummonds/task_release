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

	// --- Remotes summary ---
	printRemotes(dir)

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
	case "binary", "library":
		findings = append(findings, finding{levelOK, fmt.Sprintf("Type: %s", cfg.Type)})
	default:
		findings = append(findings, finding{levelError, fmt.Sprintf("Invalid type %q (expected: binary, library)", cfg.Type)})
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

func printRemotes(dir string) {
	cfg, err := config.Load(dir)
	if err != nil || len(cfg.Remotes) == 0 {
		return
	}

	fmt.Println("\nRemotes (push targets):")
	for _, name := range cfg.Remotes {
		url, err := git.RemoteURL(dir, name)
		if err != nil {
			fmt.Printf("  %-16s (not found in git)\n", name)
			continue
		}
		forgeType := forge.DetectFromURL(url)
		hasCLI := forge.Forge{Type: forgeType}.HasCLI()
		extra := ""
		if hasCLI {
			extra = ", cli: yes"
		}
		fmt.Printf("  %-16s %s (%s%s)\n", name, url, forgeType, extra)
	}
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
