package pages

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxPort = 8090

// Serve runs optional build commands, then starts an HTTP file server for docs/.
// It tries ports from the given port up to maxPort.
func Serve(dir string, port int, buildCmds []string) error {
	// Run build commands first
	for _, cmd := range buildCmds {
		fmt.Printf("$ %s\n", cmd)
		parts := strings.Fields(cmd)
		c := exec.Command(parts[0], parts[1:]...)
		c.Dir = dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", cmd, err)
		}
	}

	docsDir := filepath.Join(dir, "docs")
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		return fmt.Errorf("docs/ directory not found in %s", dir)
	}

	handler := http.FileServer(http.Dir(docsDir))

	for p := port; p <= maxPort; p++ {
		addr := fmt.Sprintf(":%d", p)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			fmt.Printf("  Port %d in use, trying next...\n", p)
			continue
		}
		fmt.Printf("Serving %s on http://localhost%s\n", docsDir, addr)
		return http.Serve(ln, handler)
	}

	return fmt.Errorf("no available port found in range %d-%d", port, maxPort)
}
