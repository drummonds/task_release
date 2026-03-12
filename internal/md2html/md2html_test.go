package md2html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsRootURL(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	research := filepath.Join(docs, "research")
	os.MkdirAll(research, 0755)

	os.WriteFile(filepath.Join(docs, "index.md"), []byte("# Index"), 0644)

	if got := docsRootURL(docs); got != "index.html" {
		t.Errorf("docsRootURL(docs) = %q, want %q", got, "index.html")
	}

	if got := docsRootURL(research); got != "../index.html" {
		t.Errorf("docsRootURL(research) = %q, want %q", got, "../index.html")
	}
}

func TestMarkerReplacementEndToEnd(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out")
	os.MkdirAll(dst, 0755)

	// Create an existing HTML page so auto:pages has something to list.
	os.WriteFile(filepath.Join(dst, "guide.html"),
		[]byte("<html><head><title>Guide</title></head><body></body></html>"), 0644)

	// Create a markdown file with markers.
	md := "# My Docs\n\n<!-- auto:pages -->\n<!-- /auto:pages -->\n\nSome text.\n"
	os.WriteFile(filepath.Join(dir, "index.md"), []byte(md), 0644)

	cfg := Config{
		Src:  dir,
		Dst:  dst,
		File: filepath.Join(dir, "index.md"),
	}
	if err := Run(cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(dst, "index.html"))
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	html := string(out)

	if !strings.Contains(html, "Guide") {
		t.Errorf("expected Guide in output, got:\n%s", html)
	}
	if !strings.Contains(html, "guide.html") {
		t.Errorf("expected guide.html link in output, got:\n%s", html)
	}
	if !strings.Contains(html, "Some text.") {
		t.Errorf("expected hand-written content preserved, got:\n%s", html)
	}
}
