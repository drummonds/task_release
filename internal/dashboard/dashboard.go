// Package dashboard provides terminal and web dashboards for monitoring worktree agents.
package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"codeberg.org/hum3/task-plus/internal/agent"
)

// Run dispatches dashboard subcommands.
func Run(args []string) error {
	term := false
	for _, a := range args {
		if a == "--term" {
			term = true
		}
	}
	if term {
		return runTerm()
	}
	return runWeb()
}

// pollAgent queries a single agent's /status endpoint. Returns a stub on failure.
func pollAgent(entry agent.AgentEntry) agent.StatusResponse {
	url := fmt.Sprintf("http://localhost:%d/status", entry.Port)
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return agent.StatusResponse{
			Branch:     entry.Branch,
			LastCommit: "—",
		}
	}
	defer func() { _ = resp.Body.Close() }()

	var sr agent.StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return agent.StatusResponse{
			Branch:     entry.Branch,
			LastCommit: "—",
		}
	}
	return sr
}
