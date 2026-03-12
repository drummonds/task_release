package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/drummonds/task-plus/internal/check"
	"github.com/drummonds/task-plus/internal/claude"
	"github.com/drummonds/task-plus/internal/combine"
	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/deploy"
	"github.com/drummonds/task-plus/internal/forge"
	"github.com/drummonds/task-plus/internal/git"
	"github.com/drummonds/task-plus/internal/md2html"
	"github.com/drummonds/task-plus/internal/mdupdate"
	"github.com/drummonds/task-plus/internal/pages"
	"github.com/drummonds/task-plus/internal/prompt"
	"github.com/drummonds/task-plus/internal/readme"
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
	{"check", "Validate task-plus.yml and Taskfile.yml configuration"},
	{"release", "Interactive release workflow (runs Taskfile post:release if present)"},
	{"release:version-update", "Scaffold a Taskfile task to update version strings (--init)"},
	{"repos", "Manage git remotes for release (info, add, remove)"},
	{"pages", "Serve docs/ directory over HTTP (subcommands: deploy, config, combine)"},
	{"md2html", "Convert markdown files to Bulma-styled HTML"},
	{"md_update", "Update auto-marker sections in a markdown file (toc, pages, links)"},
	{"readme", "Update auto-marker sections in README.md (links, version)"},
	{"wt", "Manage git worktrees (start, agent, review, merge, clean, list, dashboard)"},
	{"claude", "Run claude --dangerously-skip-permissions (requires worktree + sandbox)"},
	{"self", "Manage task-plus itself (update, etc.)"},
}

// Main is the entry point for both task-plus and tp binaries.
func Main() {
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
	case "--init":
		runInit()
	case "check":
		runCheck(os.Args[2:])
	case "release":
		runRelease(os.Args[2:])
	case "pages":
		runPages(os.Args[2:])
	case "release:version-update":
		runReleaseVersionUpdate(os.Args[2:])
	case "repos":
		runRepos(os.Args[2:])
	case "md2html":
		runMd2html(os.Args[2:])
	case "md_update":
		runMdUpdate(os.Args[2:])
	case "readme":
		runReadme(os.Args[2:])
	case "wt":
		runWt(os.Args[2:])
	case "claude":
		runClaude(os.Args[2:])
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
	fmt.Fprintf(os.Stderr, "  --init     Create a default task-plus.yml config file\n")
	fmt.Fprintf(os.Stderr, "  -a         List available commands\n")
	fmt.Fprintf(os.Stderr, "  --version  Print version\n")
}

func listCommands() {
	fmt.Println("task-plus commands:")
	for _, c := range commands {
		fmt.Printf("  %-24s %s\n", c.name, c.desc)
	}
}

func runInit() {
	absDir, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := config.Init(absDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	dir := fs.String("dir", ".", "project directory")
	fs.Parse(args)

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("task-plus check %s\n\n", appVersion)
	if err := check.Run(absDir); err != nil {
		os.Exit(1)
	}
}

func runRelease(args []string) {
	fs := flag.NewFlagSet("release", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "show what would happen without making changes")
	yes := fs.Bool("yes", false, "auto-confirm all prompts")
	dir := fs.String("dir", ".", "project directory")
	comment := fs.String("comment", "", "default release comment (overrides .tp-release-comment)")
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

	if err := workflow.Run(cfg, *dryRun, *comment); err != nil {
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
	// Check for subcommands before flag parsing
	if len(args) > 0 {
		switch args[0] {
		case "deploy":
			runPagesDeploy(args[1:])
			return
		case "config":
			runPagesConfig(args[1:])
			return
		case "combine":
			runPagesCombine(args[1:])
			return
		}
	}

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

func runPagesDeploy(args []string) {
	fs := flag.NewFlagSet("pages deploy", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "show what would happen without deploying")
	dir := fs.String("dir", ".", "project directory")
	fs.Parse(args)

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if !cfg.HasPagesDeploy() {
		fmt.Fprintf(os.Stderr, "No pages_deploy targets configured in task-plus.yml\n")
		os.Exit(1)
	}

	docsDir := filepath.Join(absDir, "docs")
	for _, target := range cfg.PagesDeploy {
		d, err := deploy.New(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deploying to %s...\n", d.Name())
		if err := d.Deploy(absDir, docsDir, *dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
	if !*dryRun {
		fmt.Println("Done.")
	}
}

func runPagesConfig(args []string) {
	fs := flag.NewFlagSet("pages config", flag.ExitOnError)
	dir := fs.String("dir", ".", "project directory")
	fs.Parse(args)

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if !cfg.HasPagesDeploy() {
		fmt.Println("No pages_deploy targets configured.")
		return
	}

	fmt.Println("Configured deploy targets:")
	for i, t := range cfg.PagesDeploy {
		fmt.Printf("  %d. type: %s", i+1, t.Type)
		if t.Site != "" {
			fmt.Printf(", site: %s", t.Site)
		}
		fmt.Println()
	}
}

func runPagesCombine(args []string) {
	fs := flag.NewFlagSet("pages combine", flag.ExitOnError)
	dir := fs.String("dir", ".", "project directory")
	fs.Parse(args)

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := combine.Run(absDir); err != nil {
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
	file := fs.String("file", "", "single markdown file to convert (overrides --src)")
	fs.Parse(args)

	cfg := md2html.Config{
		Src:     *src,
		Dst:     *dst,
		Label:   *label,
		Project: *project,
		File:    *file,
	}
	if err := md2html.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runMdUpdate(args []string) {
	fs := flag.NewFlagSet("md_update", flag.ExitOnError)
	dst := fs.String("dst", "", "directory to scan for HTML pages (default: file's directory)")
	fs.Parse(args)

	file := fs.Arg(0)
	if file == "" {
		fmt.Fprintf(os.Stderr, "Usage: task-plus md_update [--dst <dir>] <file.md>\n")
		os.Exit(1)
	}

	opts := mdupdate.Options{}
	if *dst != "" {
		opts.PagesDir = *dst
	}

	if err := mdupdate.Update(file, opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runReadme(args []string) {
	fs := flag.NewFlagSet("readme", flag.ExitOnError)
	ver := fs.String("version", "", "version string to insert (e.g. v0.1.46)")
	dir := fs.String("dir", ".", "project directory")
	fs.Parse(args)

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := readme.Update(absDir, *ver); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("README.md updated.")
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

func runRepos(args []string) {
	absDir, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(args) == 0 {
		reposInfo(absDir)
		return
	}

	switch args[0] {
	case "info":
		reposInfo(absDir)
	case "add":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: task-plus repos add <remote-name>\n")
			os.Exit(1)
		}
		reposAdd(absDir, args[1])
	case "remove":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: task-plus repos remove <remote-name>\n")
			os.Exit(1)
		}
		reposRemove(absDir, args[1])
	default:
		fmt.Fprintf(os.Stderr, "Unknown repos subcommand: %s\n", args[0])
		fmt.Fprintf(os.Stderr, "Usage: task-plus repos [info|add|remove]\n")
		os.Exit(1)
	}
}

func reposInfo(dir string) {
	cfg, err := config.Load(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Show configured remotes
	configured := make(map[string]bool)
	fmt.Println("Configured remotes:")
	for _, name := range cfg.Remotes {
		configured[name] = true
		url, err := git.RemoteURL(dir, name)
		if err != nil {
			fmt.Printf("  %-16s (not found in git)\n", name)
			continue
		}
		forgeType := forge.DetectFromURL(url)
		hasCLI := forge.Forge{Type: forgeType}.HasCLI()
		cli := ""
		if hasCLI {
			cli = ", cli: yes"
		}
		fmt.Printf("  %-16s %s (%s%s)\n", name, url, forgeType, cli)
	}

	// Show git remotes not yet configured
	allRemotes, err := git.Run(dir, "remote")
	if err != nil {
		return
	}
	var unconfigured []string
	for _, name := range strings.Split(allRemotes, "\n") {
		name = strings.TrimSpace(name)
		if name != "" && !configured[name] {
			unconfigured = append(unconfigured, name)
		}
	}
	if len(unconfigured) > 0 {
		fmt.Println("\nAvailable git remotes (not configured):")
		for _, name := range unconfigured {
			url, err := git.RemoteURL(dir, name)
			if err != nil {
				continue
			}
			forgeType := forge.DetectFromURL(url)
			fmt.Printf("  %-16s %s (%s)\n", name, url, forgeType)
		}
		fmt.Println("\nUse 'task-plus repos add <name>' to configure.")
	}
}

func reposAdd(dir, name string) {
	if !git.HasRemote(dir, name) {
		fmt.Fprintf(os.Stderr, "Error: git remote %q does not exist.\n", name)
		fmt.Fprintf(os.Stderr, "Add it first: git remote add %s <url>\n", name)
		os.Exit(1)
	}

	if err := config.AddRemote(dir, name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	url, _ := git.RemoteURL(dir, name)
	forgeType := forge.DetectFromURL(url)
	fmt.Printf("Added remote %q (%s, %s)\n", name, url, forgeType)
}

func reposRemove(dir, name string) {
	if err := config.RemoveRemote(dir, name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Removed remote %q\n", name)
}

func runWt(args []string) {
	if err := worktree.Run(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runClaude(args []string) {
	if err := claude.Run(args); err != nil {
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
