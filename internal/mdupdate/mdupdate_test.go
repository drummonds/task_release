package mdupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePagesNav(t *testing.T) {
	dir := t.TempDir()

	for _, f := range []struct {
		name, title string
	}{
		{"alpha.html", "Alpha Page"},
		{"beta.html", "Beta Page"},
	} {
		content := "<html><head><title>" + f.title + "</title></head><body></body></html>"
		os.WriteFile(filepath.Join(dir, f.name), []byte(content), 0644)
	}
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0644)

	nav := GeneratePagesNav(dir)
	if !strings.Contains(nav, "Alpha Page") {
		t.Errorf("expected Alpha Page in nav, got:\n%s", nav)
	}
	if !strings.Contains(nav, "beta.html") {
		t.Errorf("expected beta.html in nav, got:\n%s", nav)
	}
	if strings.Contains(nav, "index.html") {
		t.Errorf("index.html should be excluded from pages nav")
	}
	if !strings.Contains(nav, `<aside class="menu">`) {
		t.Errorf("expected Bulma menu markup, got:\n%s", nav)
	}
	if !strings.Contains(nav, `<p class="menu-label">Pages</p>`) {
		t.Errorf("expected Pages group label, got:\n%s", nav)
	}
}

func TestGeneratePagesNavSubdirs(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "guide.html"),
		[]byte("<html><head><title>Guide</title></head></html>"), 0644)

	os.MkdirAll(filepath.Join(dir, "research"), 0755)
	os.WriteFile(filepath.Join(dir, "research", "results.html"),
		[]byte("<html><head><title>Results</title></head></html>"), 0644)
	os.WriteFile(filepath.Join(dir, "research", "notes.html"),
		[]byte("<html><head><title>Notes</title></head></html>"), 0644)

	nav := GeneratePagesNav(dir)
	if !strings.Contains(nav, `<p class="menu-label">Pages</p>`) {
		t.Errorf("expected Pages group, got:\n%s", nav)
	}
	if !strings.Contains(nav, `<p class="menu-label">Research</p>`) {
		t.Errorf("expected Research group, got:\n%s", nav)
	}
	if !strings.Contains(nav, `href="research/results.html"`) {
		t.Errorf("expected subdirectory path in href, got:\n%s", nav)
	}
}

func TestGeneratePagesNavEmpty(t *testing.T) {
	dir := t.TempDir()
	nav := GeneratePagesNav(dir)
	if nav != "\n" {
		t.Errorf("expected single newline for empty dir, got: %q", nav)
	}
}

func TestGenerateTOC(t *testing.T) {
	content := []byte("# Title\n\n## Getting Started\n\n### Install\n\nSome text.\n\n## Usage\n\n### Advanced Config\n")
	toc := GenerateTOC(content)
	if !strings.Contains(toc, "[Getting Started](#getting-started)") {
		t.Errorf("expected Getting Started link, got:\n%s", toc)
	}
	if !strings.Contains(toc, "  - [Install](#install)") {
		t.Errorf("expected indented Install link, got:\n%s", toc)
	}
	if !strings.Contains(toc, "[Usage](#usage)") {
		t.Errorf("expected Usage link, got:\n%s", toc)
	}
	if strings.Contains(toc, "Title") {
		t.Errorf("h1 should be excluded from TOC, got:\n%s", toc)
	}
}

func TestHeadingToID(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Getting Started", "getting-started"},
		{"Install & Configure", "install--configure"},
		{"tp md2html", "tp-md2html"},
		{"Hello, World!", "hello-world"},
	}
	for _, tc := range tests {
		got := HeadingToID(tc.input)
		if got != tc.want {
			t.Errorf("HeadingToID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestUpdate(t *testing.T) {
	dir := t.TempDir()

	// Create an HTML page so auto:pages has something to list.
	os.WriteFile(filepath.Join(dir, "guide.html"),
		[]byte("<html><head><title>Guide</title></head></html>"), 0644)

	// Markdown with markers.
	md := "# My Docs\n\n## Intro\n\n<!-- auto:toc -->\n<!-- /auto:toc -->\n\n<!-- auto:pages -->\n<!-- /auto:pages -->\n"
	path := filepath.Join(dir, "index.md")
	os.WriteFile(path, []byte(md), 0644)

	if err := Update(path, Options{PagesDir: dir}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "[Intro](#intro)") {
		t.Errorf("expected TOC entry for Intro, got:\n%s", content)
	}
	if !strings.Contains(content, "guide.html") {
		t.Errorf("expected guide.html in pages nav, got:\n%s", content)
	}
}
