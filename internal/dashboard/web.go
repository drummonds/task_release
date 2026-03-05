package dashboard

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/drummonds/lofigui"
	"github.com/drummonds/task-plus/internal/agent"
)

const webStartPort = 8091
const webMaxPort = 8099

func runWeb() error {
	app := lofigui.NewApp()
	app.Version = "task-plus agent dashboard"
	app.SetRefreshTime(2)
	app.SetDisplayURL("/")

	ctrl, err := lofigui.NewControllerWithLayout(lofigui.LayoutNavbar, "Agent Dashboard")
	if err != nil {
		return fmt.Errorf("create controller: %w", err)
	}
	app.SetController(ctrl)
	app.StartAction() // Enable permanent polling/refresh

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		renderWebDashboard()
		app.HandleDisplay(w, r)
	})

	http.HandleFunc("/favicon.ico", lofigui.ServeFavicon)

	for p := webStartPort; p <= webMaxPort; p++ {
		addr := fmt.Sprintf(":%d", p)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		log.Printf("Agent dashboard on http://localhost%s", addr)
		return http.Serve(ln, nil)
	}
	return fmt.Errorf("no available port in range %d-%d", webStartPort, webMaxPort)
}

func renderWebDashboard() {
	agent.CleanStale()
	reg, err := agent.Load()
	if err != nil {
		lofigui.Printf("<p class=\"has-text-danger\">Error loading registry: %v</p>", err)
		return
	}

	if len(reg.Agents) == 0 {
		lofigui.HTML(`<p class="has-text-grey-light">No agents registered.</p>`)
		return
	}

	header := []string{"Task", "Branch", "Status", "Last Commit", "Uptime", "Port"}
	var rows [][]string
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
		if len(commit) > 40 {
			commit = commit[:40] + ".."
		}
		rows = append(rows, []string{
			key, sr.Branch, status, commit, uptime, fmt.Sprintf("%d", entry.Port),
		})
	}
	lofigui.Table(rows, lofigui.WithHeader(header))
}
