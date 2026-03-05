package dashboard

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/drummonds/task-plus/internal/agent"
)

func runTerm() error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Render immediately, then on tick
	renderTerm()
	for {
		select {
		case <-ticker.C:
			renderTerm()
		case <-sigCh:
			fmt.Println()
			return nil
		}
	}
}

func renderTerm() {
	// Clear screen
	fmt.Print("\033[2J\033[H")

	agent.CleanStale()
	reg, err := agent.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading registry: %v\n", err)
		return
	}

	if len(reg.Agents) == 0 {
		fmt.Println("No agents registered.")
		fmt.Println("\nPress Ctrl+C to exit.")
		return
	}

	// Header
	fmt.Printf("%-24s %-24s %-10s %-30s %-10s %s\n",
		"TASK", "BRANCH", "STATUS", "LAST COMMIT", "UPTIME", "PORT")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────────────────────────────────")

	for key, entry := range reg.Agents {
		sr := pollAgent(entry)
		status := "Offline"
		if sr.ClaudeRunning {
			status = "Running"
		} else if sr.Uptime != "" {
			status = "Idle"
		}
		uptime := sr.Uptime
		if uptime == "" {
			uptime = "—"
		}
		commit := sr.LastCommit
		if len(commit) > 28 {
			commit = commit[:28] + ".."
		}
		fmt.Printf("%-24s %-24s %-10s %-30s %-10s %d\n",
			key, sr.Branch, status, commit, uptime, entry.Port)
	}

	fmt.Println("\nPress Ctrl+C to exit. Refreshing every 2s...")
}
