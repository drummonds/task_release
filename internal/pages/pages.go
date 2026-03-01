package pages

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// Serve starts an HTTP file server for the docs/ subdirectory of dir.
func Serve(dir string, port int) error {
	docsDir := filepath.Join(dir, "docs")
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		return fmt.Errorf("docs/ directory not found in %s", dir)
	}

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Serving %s on http://localhost%s\n", docsDir, addr)
	return http.ListenAndServe(addr, http.FileServer(http.Dir(docsDir)))
}
