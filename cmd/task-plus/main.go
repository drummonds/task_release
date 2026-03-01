package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/pages"
	"github.com/drummonds/task-plus/internal/prompt"
	"github.com/drummonds/task-plus/internal/workflow"
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
	{"release", "Interactive release workflow"},
	{"pages", "Serve docs/ directory over HTTP"},
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "-a":
		listCommands()
	case "--version", "-version":
		fmt.Println("task-plus", appVersion)
	case "release":
		runRelease(os.Args[2:])
	case "pages":
		runPages(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: task-plus <command> [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "  %-10s %s\n", c.name, c.desc)
	}
	fmt.Fprintf(os.Stderr, "\nFlags:\n")
	fmt.Fprintf(os.Stderr, "  -a         List available commands\n")
	fmt.Fprintf(os.Stderr, "  --version  Print version\n")
}

func listCommands() {
	fmt.Println("task-plus commands:")
	for _, c := range commands {
		fmt.Printf("  %-10s %s\n", c.name, c.desc)
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
}

// hasTaskfileTask checks if YAML data contains a top-level task with the given name.
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
		if inTasks && (line == prefix || len(line) > len(prefix) && line[:len(prefix)] == prefix) {
			return true
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
