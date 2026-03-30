// Package self provides the "self" command group for task-plus management.
package self

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	moduleName = "codeberg.org/hum3/task-plus"
	modulePath = moduleName + "/cmd/task-plus"
)

// Run dispatches self sub-subcommands.
func Run(args []string, version string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: task-plus self <update>")
	}

	switch args[0] {
	case "update":
		return runUpdate(version)
	default:
		return fmt.Errorf("unknown self command: %s", args[0])
	}
}

func runUpdate(currentVersion string) error {
	fmt.Printf("Current version: %s\n", currentVersion)

	latest, err := FetchLatestVersion()
	if err != nil {
		fmt.Printf("Could not check latest version: %v\n", err)
		fmt.Println("Proceeding with update anyway...")
	} else {
		fmt.Printf("Latest version:  %s\n", latest)
		if latest == currentVersion {
			fmt.Println("Already up to date.")
			return nil
		}
	}

	fmt.Printf("Installing: go install %s@latest\n", modulePath)
	cmd := exec.Command("go", "install", modulePath+"@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install: %w", err)
	}

	if err := symlinkTP(); err != nil {
		fmt.Printf("Warning: could not create tp symlink: %v\n", err)
	}

	fmt.Println("Updated successfully.")
	return nil
}

// symlinkTP creates a "tp" symlink pointing to "task-plus" in GOBIN.
func symlinkTP() error {
	binDir, err := goBinDir()
	if err != nil {
		return err
	}
	target := filepath.Join(binDir, "task-plus")
	link := filepath.Join(binDir, "tp")

	// Remove existing file/symlink so we can recreate it.
	if _, err := os.Lstat(link); err == nil {
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("remove old tp: %w", err)
		}
	}
	fmt.Printf("Symlinking: %s -> %s\n", link, target)
	return os.Symlink(target, link)
}

// goBinDir returns the directory where "go install" places binaries.
func goBinDir() (string, error) {
	if dir := os.Getenv("GOBIN"); dir != "" {
		return dir, nil
	}
	out, err := exec.Command("go", "env", "GOPATH").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOPATH: %w", err)
	}
	gopath := strings.TrimSpace(string(out))
	if gopath == "" {
		return "", fmt.Errorf("GOPATH is empty")
	}
	return filepath.Join(gopath, "bin"), nil
}

// FetchLatestVersion queries the Go module proxy for the latest version
// with a short timeout so it doesn't block startup.
func FetchLatestVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	url := "https://proxy.golang.org/" + moduleName + "/@latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("proxy returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// Response is JSON: {"Version":"v0.1.21","Time":"..."}
	s := string(body)
	if i := strings.Index(s, `"Version":"`); i >= 0 {
		s = s[i+len(`"Version":"`):]
		if j := strings.Index(s, `"`); j >= 0 {
			return s[:j], nil
		}
	}
	return "", fmt.Errorf("unexpected proxy response")
}
