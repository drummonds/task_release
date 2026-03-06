// Package worktree manages git worktrees for running Claude tasks in isolation.
package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/drummonds/task-plus/internal/agent"
	"github.com/drummonds/task-plus/internal/dashboard"
)

const settingsJSON = `{
  "sandbox": {
    "enabled": true,
    "autoAllowBashIfSandboxed": true,
    "filesystem": {
      "denyRead": ["~/.ssh", "~/.aws"]
    }
  }
}
`

// Sandbox stub files that Claude Code creates as a known bug.
var sandboxStubs = []string{
	".bashrc",
	".gitconfig",
	"HEAD",
}

// Run dispatches wt sub-subcommands.
func Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: task-plus wt <start|agent|review|merge|clean|list|dashboard|--init>")
	}

	switch args[0] {
	case "start":
		return runStart(args[1:])
	case "agent":
		return runAgent(args[1:])
	case "review":
		return runReview(args[1:])
	case "merge":
		return runMerge(args[1:])
	case "clean":
		return runClean(args[1:])
	case "list":
		return runList(args[1:])
	case "dashboard":
		return dashboard.Run(args[1:])
	case "--init":
		printInit()
		return nil
	default:
		return fmt.Errorf("unknown wt command: %s\nUsage: task-plus wt <start|agent|review|merge|clean|list|dashboard|--init>", args[0])
	}
}

// runStart creates (or resumes) a worktree and opens VS Code in it.
// It does not run claude or register with the agent dashboard.
func runStart(args []string) error {
	task, dir, err := parseTaskArgs(args)
	if err != nil {
		return err
	}

	projName, err := projectName(dir)
	if err != nil {
		return err
	}

	wtPath := worktreePath(dir, projName, task)
	branch := "task/" + task

	// Resume or create worktree
	if info, err := os.Stat(wtPath); err == nil && info.IsDir() {
		fmt.Printf("Resuming existing worktree at %s\n", wtPath)
		ensureSettings(wtPath)
	} else {
		fmt.Printf("Creating worktree at %s on branch %s\n", wtPath, branch)
		if err := git(dir, "worktree", "add", wtPath, "-b", branch); err != nil {
			return fmt.Errorf("git worktree add: %w", err)
		}
		if err := writeSettings(wtPath); err != nil {
			return err
		}
		if err := addToGitExclude(wtPath, append([]string{".claude/settings.json"}, sandboxStubs...)); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not update git exclude: %v\n", err)
		}
		addToGitignore(dir, []string{".claude/settings.json"})
	}

	// Open VS Code
	if _, err := exec.LookPath("code"); err == nil {
		fmt.Printf("Opening VS Code in %s\n", wtPath)
		exec.Command("code", wtPath).Run()
	} else {
		fmt.Printf("Worktree ready at %s (VS Code 'code' not found in PATH)\n", wtPath)
	}

	return nil
}

// runAgent registers a worktree with the agent dashboard and optionally runs claude.
// The worktree must already exist (created by 'wt start').
func runAgent(args []string) error {
	task, spec, dir, err := parseStartArgs(args)
	if err != nil {
		return err
	}

	projName, err := projectName(dir)
	if err != nil {
		return err
	}

	wtPath := worktreePath(dir, projName, task)
	branch := "task/" + task
	registryKey := projName + "/" + task

	// Check worktree exists
	if info, err := os.Stat(wtPath); err != nil || !info.IsDir() {
		return fmt.Errorf("worktree not found at %s; run 'task-plus wt start --task=%s' first", wtPath, task)
	}

	// Clean stale agents
	if removed, err := agent.CleanStale(); err == nil && len(removed) > 0 {
		for _, k := range removed {
			fmt.Printf("Cleaned stale agent: %s\n", k)
		}
	}

	// Start HTTP status server
	startTime := time.Now()
	var claudeRunning atomic.Bool
	entry := agent.AgentEntry{
		PID:          os.Getpid(),
		WorktreePath: wtPath,
		Branch:       branch,
		Project:      projName,
		StartTime:    startTime,
	}

	port, srv, err := agent.StartStatusServer(entry, &claudeRunning)
	if err != nil {
		return fmt.Errorf("start status server: %w", err)
	}
	entry.Port = port

	// Register agent
	if err := agent.Register(registryKey, entry); err != nil {
		return fmt.Errorf("register agent: %w", err)
	}
	fmt.Printf("Agent registered: %s (port %d)\n", registryKey, port)

	// Signal handling for clean shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Run claude
	var claudeErr error
	if spec != "" {
		fmt.Printf("Running claude in %s\n", wtPath)
		claudeRunning.Store(true)

		c := exec.Command("claude", spec)
		c.Dir = wtPath
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin

		doneCh := make(chan error, 1)
		go func() {
			doneCh <- c.Run()
		}()

		select {
		case claudeErr = <-doneCh:
			// Normal exit
		case sig := <-sigCh:
			// Forward signal to claude process
			if c.Process != nil {
				c.Process.Signal(sig)
			}
			// Wait for it to finish
			claudeErr = <-doneCh
		}
		claudeRunning.Store(false)
	} else {
		// No spec: wait for signal
		fmt.Println("No --spec provided; agent running. Press Ctrl+C to stop.")
		<-sigCh
	}

	// Cleanup
	signal.Stop(sigCh)
	srv.Shutdown(context.Background())
	if err := agent.Deregister(registryKey); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: deregister: %v\n", err)
	}
	fmt.Printf("Agent stopped: %s\n", registryKey)

	if claudeErr != nil {
		return fmt.Errorf("claude: %w", claudeErr)
	}
	return nil
}

func runReview(args []string) error {
	task, dir, err := parseTaskArgs(args)
	if err != nil {
		return err
	}
	branch := "task/" + task

	cmd := exec.Command("git", "-C", dir, "diff", "main..."+branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runMerge(args []string) error {
	task, dir, err := parseTaskArgs(args)
	if err != nil {
		return err
	}

	projName, err := projectName(dir)
	if err != nil {
		return err
	}

	wtPath := worktreePath(dir, projName, task)
	branch := "task/" + task

	fmt.Printf("Merging %s into current branch\n", branch)
	if err := git(dir, "merge", branch); err != nil {
		return fmt.Errorf("merge: %w", err)
	}

	fmt.Printf("Removing worktree at %s\n", wtPath)
	if err := git(dir, "worktree", "remove", wtPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: worktree remove: %v\n", err)
	}

	if err := git(dir, "branch", "-d", branch); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: branch delete: %v\n", err)
	}

	return nil
}

func runClean(args []string) error {
	task, dir, err := parseTaskArgs(args)
	if err != nil {
		return err
	}

	projName, err := projectName(dir)
	if err != nil {
		return err
	}

	wtPath := worktreePath(dir, projName, task)
	branch := "task/" + task

	fmt.Printf("Removing worktree at %s (force)\n", wtPath)
	if err := git(dir, "worktree", "remove", wtPath, "--force"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: worktree remove: %v\n", err)
	}

	if err := git(dir, "branch", "-D", branch); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: branch delete: %v\n", err)
	}

	return nil
}

func runList(args []string) error {
	dir := "."
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--dir" {
			dir = args[i+1]
		}
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	return git(absDir, "worktree", "list")
}

// parseStartArgs extracts --task, --spec, --dir from args.
func parseStartArgs(args []string) (task, spec, dir string, err error) {
	dir = "."
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--task" && i+1 < len(args):
			task = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--task="):
			task = args[i][len("--task="):]
		case args[i] == "--spec" && i+1 < len(args):
			spec = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--spec="):
			spec = args[i][len("--spec="):]
		case args[i] == "--dir" && i+1 < len(args):
			dir = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--dir="):
			dir = args[i][len("--dir="):]
		}
	}
	if task == "" {
		return "", "", "", fmt.Errorf("--task is required\nUsage: task-plus wt agent --task=<name> --spec=\"<prompt>\"")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", "", err
	}
	return task, spec, absDir, nil
}

// parseTaskArgs extracts --task and --dir from args.
func parseTaskArgs(args []string) (task, dir string, err error) {
	dir = "."
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--task" && i+1 < len(args):
			task = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--task="):
			task = args[i][len("--task="):]
		case args[i] == "--dir" && i+1 < len(args):
			dir = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--dir="):
			dir = args[i][len("--dir="):]
		}
	}
	if task == "" {
		return "", "", fmt.Errorf("--task is required")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	return task, absDir, nil
}

func projectName(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return filepath.Base(strings.TrimSpace(string(out))), nil
}

func worktreePath(dir, projName, task string) string {
	// Place worktree alongside the repo root
	topOut, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return filepath.Join(filepath.Dir(dir), projName+"-"+task)
	}
	top := strings.TrimSpace(string(topOut))
	return filepath.Join(filepath.Dir(top), projName+"-"+task)
}

func git(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeSettings(wtPath string) error {
	claudeDir := filepath.Join(wtPath, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("mkdir .claude: %w", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settingsJSON), 0644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	return nil
}

func ensureSettings(wtPath string) {
	settingsPath := filepath.Join(wtPath, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		writeSettings(wtPath)
	}
}

// addToGitExclude writes entries to the worktree's per-checkout exclude file.
func addToGitExclude(wtPath string, entries []string) error {
	out, err := exec.Command("git", "-C", wtPath, "rev-parse", "--git-dir").Output()
	if err != nil {
		return err
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(wtPath, gitDir)
	}

	excludePath := filepath.Join(gitDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0755); err != nil {
		return err
	}

	existing, _ := os.ReadFile(excludePath)
	content := string(existing)

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(content, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) > 0 {
		f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()

		if len(content) > 0 && content[len(content)-1] != '\n' {
			f.WriteString("\n")
		}
		f.WriteString("# task-plus worktree sandbox\n")
		for _, entry := range toAdd {
			f.WriteString(entry + "\n")
		}
	}
	return nil
}

// addToGitignore appends entries to .gitignore if not already present.
func addToGitignore(dir string, entries []string) {
	gitignorePath := filepath.Join(dir, ".gitignore")
	existing, _ := os.ReadFile(gitignorePath)
	content := string(existing)

	var toAdd []string
	for _, entry := range entries {
		if !strings.Contains(content, entry) {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) > 0 {
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()

		if len(content) > 0 && content[len(content)-1] != '\n' {
			f.WriteString("\n")
		}
		for _, entry := range toAdd {
			f.WriteString(entry + "\n")
		}
	}
}

func printInit() {
	fmt.Print(`# Add these to your Taskfile.yml.
# Requires task-plus to be installed.
#
# Usage:
#   task wt:start TASK=my-feature
#   task wt:agent TASK=my-feature SPEC="implement the login page"
#   task wt:review TASK=my-feature
#   task wt:merge TASK=my-feature
#   task wt:clean TASK=my-feature
#   task wt:list
#   task wt:dashboard

  wt:start:
    desc: Create a worktree and open VS Code in it
    requires:
      vars: [TASK]
    cmds:
      - task-plus wt start --task={{.TASK}}

  wt:agent:
    desc: Run Claude agent in a worktree (registers with dashboard)
    requires:
      vars: [TASK, SPEC]
    cmds:
      - task-plus wt agent --task={{.TASK}} --spec="{{.SPEC}}"

  wt:review:
    desc: Review changes in a worktree task
    requires:
      vars: [TASK]
    cmds:
      - task-plus wt review --task={{.TASK}}

  wt:merge:
    desc: Merge task branch and remove worktree
    requires:
      vars: [TASK]
    cmds:
      - task-plus wt merge --task={{.TASK}}

  wt:clean:
    desc: Remove worktree and delete branch without merging
    requires:
      vars: [TASK]
    cmds:
      - task-plus wt clean --task={{.TASK}}

  wt:list:
    desc: List active worktrees
    cmds:
      - task-plus wt list

  wt:dashboard:
    desc: Show agent dashboard (web UI; use --term for terminal)
    cmds:
      - task-plus wt dashboard
`)
}
