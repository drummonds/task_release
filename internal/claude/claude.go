// Package claude provides the "task-plus claude" command which runs
// claude --dangerously-skip-permissions with safety checks.
package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run checks that we're in a git worktree with sandbox enabled,
// then runs claude --dangerously-skip-permissions.
func Run(args []string) error {
	if err := checkWorktree(); err != nil {
		return err
	}
	if err := checkSandbox(); err != nil {
		return err
	}

	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH")
	}

	cmdArgs := append([]string{"--dangerously-skip-permissions"}, args...)
	fmt.Println("Starting claude --dangerously-skip-permissions")
	cmd := exec.Command(claudePath, cmdArgs...)
	cmd.Dir, _ = os.Getwd()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func checkWorktree() error {
	commonDir, err := gitOutput("rev-parse", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("not in a git repository")
	}
	gitDir, err := gitOutput("rev-parse", "--git-dir")
	if err != nil {
		return fmt.Errorf("not in a git repository")
	}

	absCommon, _ := filepath.Abs(commonDir)
	absGit, _ := filepath.Abs(gitDir)

	if absCommon == absGit {
		return fmt.Errorf("not in a git worktree (run from a worktree created by 'task-plus wt start')")
	}
	return nil
}

func checkSandbox() error {
	data, err := os.ReadFile(filepath.Join(".claude", "settings.json"))
	if err != nil {
		return fmt.Errorf(".claude/settings.json not found; sandbox not configured")
	}

	var settings struct {
		Sandbox struct {
			Enabled bool `json:"enabled"`
		} `json:"sandbox"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("invalid .claude/settings.json: %w", err)
	}
	if !settings.Sandbox.Enabled {
		return fmt.Errorf("sandbox is not enabled in .claude/settings.json")
	}
	return nil
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
