// Package self provides the "self" command group for task-plus management.
package self

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

const (
	modulePath = "github.com/drummonds/task-plus/cmd/task-plus"
	moduleName = "github.com/drummonds/task-plus"
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

	latest, err := fetchLatestVersion()
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

	fmt.Println("Updated successfully.")
	return nil
}

// fetchLatestVersion queries the Go module proxy for the latest version.
func fetchLatestVersion() (string, error) {
	url := "https://proxy.golang.org/" + moduleName + "/@latest"
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("proxy returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// Response is JSON: {"Version":"v0.1.21","Time":"..."}
	// Simple extraction to avoid adding encoding/json for one field.
	s := string(body)
	if i := strings.Index(s, `"Version":"`); i >= 0 {
		s = s[i+len(`"Version":"`):]
		if j := strings.Index(s, `"`); j >= 0 {
			return s[:j], nil
		}
	}
	return "", fmt.Errorf("unexpected proxy response")
}
