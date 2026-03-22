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

	"codeberg.org/hum3/task-plus/internal/agent"
	"codeberg.org/hum3/task-plus/internal/dashboard"
	"codeberg.org/hum3/task-plus/internal/prompt"
	"codeberg.org/hum3/task-plus/internal/releasecomment"
	"codeberg.org/hum3/task-plus/internal/vscode"
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

const vscodeTasksJSON = `{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Terminal 1",
      "type": "shell",
      "command": "bash",
      "isBackground": true,
      "presentation": { "reveal": "always", "panel": "dedicated", "group": "terminals" },
      "runOptions": { "runOn": "folderOpen" }
    },
    {
      "label": "Terminal 2",
      "type": "shell",
      "command": "bash",
      "isBackground": true,
      "presentation": { "reveal": "always", "panel": "dedicated", "group": "terminals" },
      "runOptions": { "runOn": "folderOpen" }
    }
  ]
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
		if err := writeVSCodeTasks(wtPath); err != nil {
			return err
		}
		if err := addToGitExclude(wtPath, append([]string{".claude/settings.json", ".vscode/tasks.json"}, sandboxStubs...)); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not update git exclude: %v\n", err)
		}
	}

	// Open VS Code — use --add to add as workspace folder instead of new window
	if _, err := exec.LookPath("code"); err == nil {
		fmt.Printf("Opening VS Code workspace folder %s\n", wtPath)
		_ = exec.Command("code", "--add", wtPath).Run()
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
		return fmt.Errorf("worktree not found at %s; run 'task-plus wt start %s' first", wtPath, task)
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
				_ = c.Process.Signal(sig)
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
	_ = srv.Shutdown(context.Background())
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
	if err := rejectIfInsideWorktree(); err != nil {
		return err
	}

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

	// Check for uncommitted changes before merging
	if dirty, _ := isDirty(dir); dirty {
		return fmt.Errorf("working tree has uncommitted changes — commit or discard them before merge")
	}

	// Capture last commit subject before merging
	commitMsg := lastCommitSubject(dir, branch)

	fmt.Printf("Merging %s into current branch\n", branch)
	if err := git(dir, "merge", branch); err != nil {
		return fmt.Errorf("merge: %w", err)
	}

	saveReleaseComment(dir, commitMsg)

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
	if err := rejectIfInsideWorktree(); err != nil {
		return err
	}

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

	// Show plan
	fmt.Println("This will:")
	fmt.Printf("  1. Merge %s into current branch\n", branch)
	fmt.Printf("  2. Close VS Code workspace folder\n")
	fmt.Printf("  3. Remove from VS Code recently opened list\n")
	fmt.Printf("  4. Remove worktree at %s\n", wtPath)
	fmt.Printf("  5. Delete branch %s\n", branch)
	fmt.Println()

	if !prompt.Confirm("Proceed?") {
		fmt.Println("Aborted.")
		return nil
	}

	// Check for uncommitted changes before merging
	if dirty, _ := isDirty(dir); dirty {
		return fmt.Errorf("working tree has uncommitted changes — commit or discard them before merge")
	}

	// Capture last commit subject before merging
	commitMsg := lastCommitSubject(dir, branch)

	// 1. Merge
	fmt.Printf("\nMerging %s into current branch\n", branch)
	if err := git(dir, "merge", branch); err != nil {
		return fmt.Errorf("merge failed: %w (worktree left in place)", err)
	}

	saveReleaseComment(dir, commitMsg)

	// 2. Close VS Code workspace folder (before removing worktree directory)
	closeVSCodeFolder(wtPath)

	// 3. Remove from VS Code recently opened list
	if err := vscode.RemoveFromRecent(wtPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: VS Code recent list: %v\n", err)
	} else {
		fmt.Printf("Removed from VS Code recently opened list\n")
	}

	// 4. Remove worktree
	fmt.Printf("Removing worktree at %s\n", wtPath)
	if err := git(dir, "worktree", "remove", wtPath, "--force"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: worktree remove: %v\n", err)
	}

	// 5. Delete branch
	if err := git(dir, "branch", "-d", branch); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: branch delete: %v\n", err)
	}

	return nil
}

// closeVSCodeFolder removes a folder from the current VS Code workspace.
func closeVSCodeFolder(wtPath string) {
	codePath, err := exec.LookPath("code")
	if err != nil {
		return
	}
	// URI-encode the path for the --remove flag
	fmt.Printf("Closing VS Code workspace folder %s\n", wtPath)
	_ = exec.Command(codePath, "--remove", wtPath).Run()
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

// parseStartArgs extracts the task name, --spec, and --dir from args.
// The task name can be given as a positional argument or via --task flag.
// A "WT" prefix is automatically added if not already present.
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
		case !strings.HasPrefix(args[i], "-") && task == "":
			task = args[i]
		}
	}
	if task == "" {
		return "", "", "", fmt.Errorf("usage: task-plus wt agent <name> --spec=\"<prompt>\"")
	}
	if !strings.HasPrefix(task, wtPrefix) {
		task = wtPrefix + task
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", "", err
	}
	return task, spec, absDir, nil
}

// wtPrefix is automatically prepended to task names so worktree directories
// (e.g. project-WTdemo) stand out from the main project directory.
const wtPrefix = "WT"

// parseTaskArgs extracts the task name and --dir from args.
// The task name can be given as a positional argument or via --task flag.
// A "WT" prefix is automatically added if not already present.
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
		case !strings.HasPrefix(args[i], "-") && task == "":
			task = args[i]
		}
	}
	if task == "" {
		return "", "", fmt.Errorf("usage: task-plus wt <command> <name>")
	}
	lower := strings.ToLower(task)
	if lower == "doc" || lower == "docs" {
		return "", "", fmt.Errorf("task name %q is reserved — it clashes with the -docs repo convention", task)
	}
	if !strings.HasPrefix(task, wtPrefix) {
		task = wtPrefix + task
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

// rejectIfInsideWorktree returns an error if cwd is inside a git worktree
// (as opposed to the main working tree). This prevents commands like clean
// and merge from operating on the tree they're standing in.
func rejectIfInsideWorktree() error {
	gitDir, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		return nil // not a git repo — let later commands handle it
	}
	commonDir, err := exec.Command("git", "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return nil
	}
	gd := strings.TrimSpace(string(gitDir))
	cd := strings.TrimSpace(string(commonDir))
	// In the main working tree these are equal (both ".git"); in a worktree
	// git-dir points to .git/worktrees/<name> while common-dir points to .git.
	if gd != cd {
		return fmt.Errorf("refusing to run inside a worktree — switch to the main project directory first")
	}
	return nil
}

// isDirty returns true if the working tree has uncommitted changes.
func isDirty(dir string) (bool, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
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

func writeVSCodeTasks(wtPath string) error {
	vscodeDir := filepath.Join(wtPath, ".vscode")
	if err := os.MkdirAll(vscodeDir, 0755); err != nil {
		return fmt.Errorf("mkdir .vscode: %w", err)
	}
	if err := os.WriteFile(filepath.Join(vscodeDir, "tasks.json"), []byte(vscodeTasksJSON), 0644); err != nil {
		return fmt.Errorf("write tasks.json: %w", err)
	}
	return nil
}

func ensureSettings(wtPath string) {
	settingsPath := filepath.Join(wtPath, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		_ = writeSettings(wtPath)
	}
	tasksPath := filepath.Join(wtPath, ".vscode", "tasks.json")
	if _, err := os.Stat(tasksPath); os.IsNotExist(err) {
		_ = writeVSCodeTasks(wtPath)
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
		defer f.Close() //nolint:errcheck // best-effort append

		if len(content) > 0 && content[len(content)-1] != '\n' {
			_, _ = f.WriteString("\n")
		}
		_, _ = f.WriteString("# task-plus worktree sandbox\n")
		for _, entry := range toAdd {
			_, _ = f.WriteString(entry + "\n")
		}
	}
	return nil
}

// lastCommitSubject returns the subject line of the last commit on branch.
func lastCommitSubject(dir, branch string) string {
	out, err := exec.Command("git", "-C", dir, "log", "-1", "--format=%s", branch).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// saveReleaseComment writes the commit message as the default release comment.
func saveReleaseComment(dir, msg string) {
	if msg == "" {
		return
	}
	if err := releasecomment.Write(dir, msg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save release comment: %v\n", err)
	} else {
		fmt.Printf("Saved release comment: %s\n", msg)
	}
}

func printInit() {
	fmt.Print(`# Add these to your Taskfile.yml.
# Requires task-plus to be installed.
#
# Usage (WT prefix is added automatically):
#   task wt:start TASK=my-feature       -> worktree WTmy-feature
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
      - task-plus wt start {{.TASK}}

  wt:agent:
    desc: Run Claude agent in a worktree (registers with dashboard)
    requires:
      vars: [TASK, SPEC]
    cmds:
      - task-plus wt agent {{.TASK}} --spec="{{.SPEC}}"

  wt:review:
    desc: Review changes in a worktree task
    requires:
      vars: [TASK]
    cmds:
      - task-plus wt review {{.TASK}}

  wt:merge:
    desc: Merge task branch and remove worktree
    requires:
      vars: [TASK]
    cmds:
      - task-plus wt merge {{.TASK}}

  wt:clean:
    desc: Merge branch, close VS Code, remove from recent list, remove worktree, delete branch
    requires:
      vars: [TASK]
    cmds:
      - task-plus wt clean {{.TASK}}

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
