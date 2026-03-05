package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/md2html"
	"github.com/drummonds/task-plus/internal/pages"
	"github.com/drummonds/task-plus/internal/prompt"
	"github.com/drummonds/task-plus/internal/self"
	"github.com/drummonds/task-plus/internal/version"
	"github.com/drummonds/task-plus/internal/workflow"
	"github.com/drummonds/task-plus/internal/worktree"
)

var (
	// Set by goreleaser via ldflags; falls back to module version from go install.
	appVersion = "dev"
)

func init() {
	if appVersion == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			appVersion = info.Main.Version
		}
	}
}

var commands = []struct {
	name string
	desc string
}{
	{"release", "Interactive release workflow (runs Taskfile post:release if present)"},
	{"release:version-update", "Scaffold a Taskfile task to update version strings (--init)"},
	{"pages", "Serve docs/ directory over HTTP"},
	{"md2html", "Convert markdown files to Bulma-styled HTML"},
	{"wt", "Manage git worktrees for Claude tasks (start, review, merge, clean, list, dashboard)"},
	{"self", "Manage task-plus itself (update, etc.)"},
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	checkForUpdate()

	switch os.Args[1] {
	case "-a":
		listCommands()
	case "--version", "-version":
		fmt.Println("task-plus", appVersion)
	case "release":
		runRelease(os.Args[2:])
	case "pages":
		runPages(os.Args[2:])
	case "release:version-update":
		runReleaseVersionUpdate(os.Args[2:])
	case "md2html":
		runMd2html(os.Args[2:])
	case "wt":
		runWt(os.Args[2:])
	case "self":
		runSelf(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

// checkForUpdate silently checks the Go module proxy and prints a hint if a newer version exists.
func checkForUpdate() {
	if appVersion == "dev" || !strings.HasPrefix(appVersion, "v") {
		return
	}
	current, err := version.Parse(appVersion)
	if err != nil {
		return
	}
	latest, err := self.FetchLatestVersion()
	if err != nil {
		return
	}
	lv, err := version.Parse(latest)
	if err != nil {
		return
	}
	if current.Less(lv) {
		fmt.Fprintf(os.Stderr, "Update available: %s -> %s (run: task-plus self update)\n", appVersion, latest)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: task-plus <command> [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "  %-24s %s\n", c.name, c.desc)
	}
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	fmt.Fprintf(os.Stderr, "  -a         List available commands\n")
	fmt.Fprintf(os.Stderr, "  --version  Print version\n")
}

func listCommands() {
	fmt.Println("task-plus commands:")
	for _, c := range commands {
		fmt.Printf("  %-24s %s\n", c.name, c.desc)
	}
}

func runRelease(args []string) {
	fs := flag.NewFlagSet("release", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "show what would happen without making changes")
	yes := fs.Bool("yes", false, "auto-confirm all prompts")
	dir := fs.String("dir", ".", "project directory")
	fs.Parse(args)

	if *yes {
		prompt.AutoConfirm = true
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Guard: refuse if Taskfile.yml has a release: task (avoid self-conflict)
	if _, serr := os.Stat(filepath.Join(absDir, "Taskfile.yml")); serr == nil {
		data, rerr := os.ReadFile(filepath.Join(absDir, "Taskfile.yml"))
		if rerr == nil && hasTaskfileTask(data, "release") {
			fmt.Fprintf(os.Stderr, "Error: Taskfile.yml contains a 'release' task.\n")
			fmt.Fprintf(os.Stderr, "Remove it to avoid conflict with 'task-plus release'.\n")
			os.Exit(1)
		}
	}

	cfg, err := config.Load(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("task-plus release %s\n", appVersion)
	fmt.Printf("Project: %s (%s)\n", absDir, cfg.Type)

	if err := workflow.Run(cfg, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	// Run post:release Taskfile task if it exists
	taskfilePath := filepath.Join(absDir, "Taskfile.yml")
	if data, err := os.ReadFile(taskfilePath); err == nil && hasTaskfileTask(data, "post:release") {
		fmt.Println("\nRunning post:release task...")
		cmd := exec.Command("task", "post:release")
		cmd.Dir = absDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "post:release failed: %v\n", err)
			os.Exit(1)
		}
	}
}

// hasTaskfileTask checks if YAML data contains a top-level task with the given name.
// Matches "  release:" but not "  release:post:" (colon-namespaced tasks are distinct).
func hasTaskfileTask(data []byte, taskName string) bool {
	prefix := "  " + taskName + ":"
	lines := splitLines(string(data))
	inTasks := false
	for _, line := range lines {
		if line == "tasks:" {
			inTasks = true
			continue
		}
		if inTasks && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inTasks = false
		}
		if inTasks && len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			// Exact match or followed by space (inline YAML), not more name chars
			if len(line) == len(prefix) || line[len(prefix)] == ' ' {
				return true
			}
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func runPages(args []string) {
	fs := flag.NewFlagSet("pages", flag.ExitOnError)
	port := fs.Int("port", 8080, "HTTP port")
	dir := fs.String("dir", ".", "project directory")
	fs.Parse(args)

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Guard: refuse if Taskfile.yml has a pages: task (avoid self-conflict)
	if _, serr := os.Stat(filepath.Join(absDir, "Taskfile.yml")); serr == nil {
		data, rerr := os.ReadFile(filepath.Join(absDir, "Taskfile.yml"))
		if rerr == nil && hasTaskfileTask(data, "pages") {
			fmt.Fprintf(os.Stderr, "Error: Taskfile.yml contains a 'pages' task.\n")
			fmt.Fprintf(os.Stderr, "Remove it to avoid conflict with 'task-plus pages'.\n")
			os.Exit(1)
		}
	}

	cfg, err := config.Load(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := pages.Serve(absDir, *port, cfg.PagesBuild); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runMd2html(args []string) {
	fs := flag.NewFlagSet("md2html", flag.ExitOnError)
	src := fs.String("src", "docs/internal", "source markdown directory")
	dst := fs.String("dst", "docs/internal", "destination HTML directory")
	label := fs.String("label", "Internal Docs", "breadcrumb label for this doc set")
	project := fs.String("project", "", "project name (auto-detected from go.mod if empty)")
	fs.Parse(args)

	cfg := md2html.Config{
		Src:     *src,
		Dst:     *dst,
		Label:   *label,
		Project: *project,
	}
	if err := md2html.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runReleaseVersionUpdate(args []string) {
	fs := flag.NewFlagSet("release:version-update", flag.ExitOnError)
	init := fs.Bool("init", false, "generate a sample release:version-update task for Taskfile.yml")
	fs.Parse(args)

	if !*init {
		fmt.Fprintf(os.Stderr, "Usage: task-plus release:version-update --init\n")
		fmt.Fprintf(os.Stderr, "\nGenerates a sample Taskfile task that updates version strings.\n")
		fmt.Fprintf(os.Stderr, "During 'task-plus release', if a release:version-update task exists\n")
		fmt.Fprintf(os.Stderr, "in your Taskfile, it will be called with VERSION=vX.Y.Z.\n")
		os.Exit(1)
	}

	fmt.Print(`# Add this to your Taskfile.yml under tasks:
# During 'task-plus release', this task is called with VERSION env var
# after the version is confirmed and before the changelog is updated.

  release:version-update:
    desc: Update version strings in project files
    cmds:
      - sed -i 's/const Version = ".*"/const Version = "{{.VERSION}}"/' version.go
    env:
      VERSION: '{{.CLI_ARGS}}'
`)
	fmt.Println()
	fmt.Println("# Adapt the sed pattern and file path to your project.")
	fmt.Println("# The VERSION environment variable is set automatically by task-plus release.")
	fmt.Println("# Example: VERSION=v0.2.0 task release:version-update")
}

func runWt(args []string) {
	if err := worktree.Run(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runSelf(args []string) {
	if err := self.Run(args, appVersion); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
