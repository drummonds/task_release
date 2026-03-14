package combine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_CombinesDocsContent(t *testing.T) {
	parent := t.TempDir()
	projectDir := filepath.Join(parent, "myproject")
	docsRepoDir := filepath.Join(parent, "myproject-docs")

	// Set up main project
	_ = os.MkdirAll(projectDir, 0755)
	_ = os.WriteFile(filepath.Join(projectDir, "task-plus.yml"), []byte("type: library\n"), 0644)

	// Set up -docs repo with content
	_ = os.MkdirAll(filepath.Join(docsRepoDir, "docs"), 0755)
	_ = os.WriteFile(filepath.Join(docsRepoDir, "task-plus.yml"), []byte("type: docs\nparent_repo: ../myproject\npages_deploy:\n  - type: statichost\n    site: h3-myproject\n"), 0644)
	_ = os.WriteFile(filepath.Join(docsRepoDir, "docs", "index.html"), []byte("<html>docs</html>"), 0644)
	_ = os.WriteFile(filepath.Join(docsRepoDir, "DOC-README.md"), []byte("# Docs README"), 0644)

	if err := Run(projectDir); err != nil {
		t.Fatal(err)
	}

	// Verify docs/ copied
	if _, err := os.Stat(filepath.Join(projectDir, "docs", "index.html")); err != nil {
		t.Error("docs/index.html not copied")
	}

	// Verify DOC- file renamed and copied
	if _, err := os.Stat(filepath.Join(projectDir, "README.md")); err != nil {
		t.Error("DOC-README.md not copied as README.md")
	}

	// Verify pages_deploy merged into main config
	data, err := os.ReadFile(filepath.Join(projectDir, "task-plus.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "statichost") {
		t.Error("pages_deploy not merged into main config")
	}
}

func TestRun_DocsProjectErrors(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("type: docs\nparent_repo: ../foo\n"), 0644)

	err := Run(dir)
	if err == nil {
		t.Fatal("expected error for docs project")
	}
}

func TestRun_NoDocsRepoErrors(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("type: library\n"), 0644)

	err := Run(dir)
	if err == nil {
		t.Fatal("expected error when no -docs sibling")
	}
}

func TestRun_SkipsExistingFiles(t *testing.T) {
	parent := t.TempDir()
	projectDir := filepath.Join(parent, "myproject")
	docsRepoDir := filepath.Join(parent, "myproject-docs")

	// Set up main project with existing README.md
	_ = os.MkdirAll(projectDir, 0755)
	_ = os.WriteFile(filepath.Join(projectDir, "task-plus.yml"), []byte("type: library\n"), 0644)
	_ = os.WriteFile(filepath.Join(projectDir, "README.md"), []byte("# Original"), 0644)

	// Set up -docs repo
	_ = os.MkdirAll(filepath.Join(docsRepoDir, "docs"), 0755)
	_ = os.WriteFile(filepath.Join(docsRepoDir, "task-plus.yml"), []byte("type: docs\nparent_repo: ../myproject\n"), 0644)
	_ = os.WriteFile(filepath.Join(docsRepoDir, "DOC-README.md"), []byte("# Docs version"), 0644)

	if err := Run(projectDir); err != nil {
		t.Fatal(err)
	}

	// Original README.md should be preserved
	data, err := os.ReadFile(filepath.Join(projectDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Original" {
		t.Errorf("README.md was overwritten: %s", data)
	}
}
