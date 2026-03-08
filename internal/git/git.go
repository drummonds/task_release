package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Run executes a git command in the given directory and returns stdout.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// Status returns the output of git status --short.
func Status(dir string) (string, error) {
	return Run(dir, "status", "--short")
}

// IsClean returns true if the working tree has no changes.
func IsClean(dir string) (bool, error) {
	out, err := Status(dir)
	if err != nil {
		return false, err
	}
	return out == "", nil
}

// AddAll stages all changes.
func AddAll(dir string) error {
	_, err := Run(dir, "add", "-A")
	return err
}

// Commit creates a commit with the given message.
func Commit(dir, msg string) error {
	_, err := Run(dir, "commit", "-m", msg)
	return err
}

// Tag creates an annotated tag.
func Tag(dir, tag, msg string) error {
	_, err := Run(dir, "tag", "-a", tag, "-m", msg)
	return err
}

// TagExists returns true if the tag already exists.
func TagExists(dir, tag string) (bool, error) {
	out, err := Run(dir, "tag", "-l", tag)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == tag, nil
}

// Tags returns all tags.
func Tags(dir string) ([]string, error) {
	out, err := Run(dir, "tag", "-l")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// Push pushes the current branch and tags to the default remote.
func Push(dir string) error {
	_, err := Run(dir, "push")
	if err != nil {
		return err
	}
	_, err = Run(dir, "push", "--tags")
	return err
}

// PushTo pushes the current branch and tags to a named remote.
func PushTo(dir, remote string) error {
	_, err := Run(dir, "push", remote)
	if err != nil {
		return err
	}
	_, err = Run(dir, "push", remote, "--tags")
	return err
}

// RemoteURL returns the URL for the named git remote.
func RemoteURL(dir, remote string) (string, error) {
	return Run(dir, "remote", "get-url", remote)
}

// HasRemote returns true if the named remote exists.
func HasRemote(dir, remote string) bool {
	_, err := RemoteURL(dir, remote)
	return err == nil
}

// Remotes returns the names of all configured git remotes.
func Remotes(dir string) ([]string, error) {
	out, err := Run(dir, "remote")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// CurrentBranch returns the current branch name.
func CurrentBranch(dir string) (string, error) {
	return Run(dir, "rev-parse", "--abbrev-ref", "HEAD")
}
