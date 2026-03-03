// Package self provides the "self" command group for task-plus management.
package self

import (
	"fmt"
	"os"
	"os/exec"
)

const modulePath = "github.com/drummonds/task-plus/cmd/task-plus"

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
	fmt.Printf("Installing latest: go install %s@latest\n", modulePath)

	cmd := exec.Command("go", "install", modulePath+"@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go install: %w", err)
	}

	fmt.Println("Updated successfully.")
	return nil
}
