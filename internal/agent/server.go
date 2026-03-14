package agent

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

// StartStatusServer starts an HTTP server on a random free port serving GET /status.
// Returns the chosen port, the server (for shutdown), and any error.
func StartStatusServer(entry AgentEntry, claudeRunning *atomic.Bool) (int, *http.Server, error) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, nil, fmt.Errorf("listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		resp := StatusResponse{
			Branch:        entry.Branch,
			LastCommit:    lastCommit(entry.WorktreePath),
			ClaudeRunning: claudeRunning.Load(),
			Uptime:        time.Since(entry.StartTime).Truncate(time.Second).String(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()

	return port, srv, nil
}

// lastCommit returns the subject of the latest commit in the worktree.
func lastCommit(wtPath string) string {
	out, err := exec.Command("git", "-C", wtPath, "log", "-1", "--format=%s").Output()
	if err != nil {
		return "—"
	}
	return strings.TrimSpace(string(out))
}
