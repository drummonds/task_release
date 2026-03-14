// Package releasecomment reads and writes the .tp-release-comment file
// that passes a worktree's last commit message to the next release.
package releasecomment

import (
	"os"
	"path/filepath"
	"strings"
)

const filename = ".tp-release-comment"

// Write stores msg in <dir>/.tp-release-comment.
func Write(dir, msg string) error {
	return os.WriteFile(filepath.Join(dir, filename), []byte(msg), 0644)
}

// Read returns the stored comment and removes the file.
// Returns ("", nil) if the file does not exist.
func Read(dir string) (string, error) {
	p := filepath.Join(dir, filename)
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	_ = os.Remove(p)
	return strings.TrimSpace(string(data)), nil
}
