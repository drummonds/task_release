package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDocsRepo_Convention(t *testing.T) {
	parent := t.TempDir()
	projectDir := filepath.Join(parent, "myproject")
	docsDir := filepath.Join(parent, "myproject-docs")
	_ = os.MkdirAll(projectDir, 0755)
	_ = os.MkdirAll(docsDir, 0755)
	_ = os.WriteFile(filepath.Join(docsDir, "task-plus.yml"), []byte("type: docs\n"), 0644)

	cfg := &Config{Dir: projectDir}
	got := cfg.ResolveDocsRepo()
	if got != docsDir {
		t.Errorf("ResolveDocsRepo() = %q, want %q", got, docsDir)
	}
}

func TestResolveDocsRepo_Explicit(t *testing.T) {
	parent := t.TempDir()
	projectDir := filepath.Join(parent, "myproject")
	customDocs := filepath.Join(parent, "custom-docs")
	_ = os.MkdirAll(projectDir, 0755)
	_ = os.MkdirAll(customDocs, 0755)
	_ = os.WriteFile(filepath.Join(customDocs, "task-plus.yml"), []byte("type: docs\n"), 0644)

	cfg := &Config{Dir: projectDir, DocsRepo: "../custom-docs"}
	got := cfg.ResolveDocsRepo()
	if got != customDocs {
		t.Errorf("ResolveDocsRepo() = %q, want %q", got, customDocs)
	}
}

func TestResolveDocsRepo_NotFound(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Dir: dir}
	got := cfg.ResolveDocsRepo()
	if got != "" {
		t.Errorf("ResolveDocsRepo() = %q, want empty", got)
	}
}

func TestResolveParentRepo_Convention(t *testing.T) {
	parent := t.TempDir()
	projectDir := filepath.Join(parent, "myproject")
	docsDir := filepath.Join(parent, "myproject-docs")
	_ = os.MkdirAll(projectDir, 0755)
	_ = os.MkdirAll(docsDir, 0755)
	_ = os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/myproject\n"), 0644)

	cfg := &Config{Dir: docsDir}
	got := cfg.ResolveParentRepo()
	if got != projectDir {
		t.Errorf("ResolveParentRepo() = %q, want %q", got, projectDir)
	}
}

func TestResolveParentRepo_NotDocsSuffix(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Dir: filepath.Join(dir, "myproject")}
	got := cfg.ResolveParentRepo()
	if got != "" {
		t.Errorf("ResolveParentRepo() = %q, want empty", got)
	}
}

func TestIsDocs(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("type: docs\nparent_repo: ../foo\n"), 0644)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.IsDocs() {
		t.Error("expected IsDocs() = true")
	}
}

func TestTypeDocsAutoDetect(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("parent_repo: ../foo\n"), 0644)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Type != "docs" {
		t.Errorf("Type = %q, want docs", cfg.Type)
	}
}

func TestHasDocsDir(t *testing.T) {
	dir := t.TempDir()
	if HasDocsDir(dir) {
		t.Error("HasDocsDir should be false for empty dir")
	}
	_ = os.MkdirAll(filepath.Join(dir, "docs"), 0755)
	if !HasDocsDir(dir) {
		t.Error("HasDocsDir should be true when docs/ exists")
	}
}
