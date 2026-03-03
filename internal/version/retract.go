package version

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// ModulePath reads go.mod in dir and returns the module path.
func ModulePath(dir string) (string, error) {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("open go.mod: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no module directive in go.mod")
}

// ParseRetracted reads go.mod in dir and returns all retracted versions.
func ParseRetracted(dir string) ([]Version, error) {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, nil // no go.mod → no retractions
	}
	defer f.Close()

	var retracted []Version
	inBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "retract (") {
			inBlock = true
			continue
		}
		if inBlock && line == ")" {
			inBlock = false
			continue
		}

		// Single-line: retract v1.2.3 // comment
		if strings.HasPrefix(line, "retract ") && !inBlock {
			ver := extractVersion(strings.TrimPrefix(line, "retract "))
			if v, err := Parse(ver); err == nil {
				retracted = append(retracted, v)
			}
			continue
		}

		// Inside block: v1.2.3 // comment
		if inBlock {
			ver := extractVersion(line)
			if v, err := Parse(ver); err == nil {
				retracted = append(retracted, v)
			}
		}
	}
	return retracted, scanner.Err()
}

// extractVersion pulls the version string from a retract line,
// stripping comments and whitespace.
func extractVersion(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "//"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// IsRetracted checks if v is in the retracted set.
func IsRetracted(v Version, retracted []Version) bool {
	return slices.Contains(retracted, v)
}

// GitRemoteModulePath derives a Go module-style path from git remote origin.
// Returns "", nil if no remote or not a recognisable URL.
func GitRemoteModulePath(dir string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", nil // no remote
	}
	return ParseGitURL(strings.TrimSpace(string(out))), nil
}

// ParseGitURL converts a git remote URL to a Go module-style path.
// Handles SSH (git@github.com:user/repo.git) and HTTPS (https://github.com/user/repo.git).
// Returns "" for unrecognised formats.
func ParseGitURL(raw string) string {
	// SSH: git@github.com:user/repo.git
	if strings.HasPrefix(raw, "git@") {
		raw = strings.TrimPrefix(raw, "git@")
		raw = strings.TrimSuffix(raw, ".git")
		// github.com:user/repo -> github.com/user/repo
		return strings.Replace(raw, ":", "/", 1)
	}
	// HTTPS: https://github.com/user/repo.git
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(raw, scheme) {
			raw = strings.TrimPrefix(raw, scheme)
			raw = strings.TrimSuffix(raw, ".git")
			return raw
		}
	}
	return ""
}
