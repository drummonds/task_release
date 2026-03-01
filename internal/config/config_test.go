package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	// No goreleaser → library
	if cfg.Type != "library" {
		t.Errorf("Type = %q, want library", cfg.Type)
	}

	// No Taskfile → fallback checks
	if len(cfg.Check) != 3 {
		t.Errorf("Check = %v, want 3 fallback commands", cfg.Check)
	}

	if cfg.ChangelogFormat != "keepachangelog" {
		t.Errorf("ChangelogFormat = %q, want keepachangelog", cfg.ChangelogFormat)
	}

	if cfg.Cleanup.KeepPatches != 2 {
		t.Errorf("KeepPatches = %d, want 2", cfg.Cleanup.KeepPatches)
	}
}

func TestDetectBinary(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".goreleaser.yaml"), []byte("{}"), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Type != "binary" {
		t.Errorf("Type = %q, want binary", cfg.Type)
	}
}

func TestDetectTaskfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte("{}"), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Check) != 1 || cfg.Check[0] != "task check" {
		t.Errorf("Check = %v, want [task check]", cfg.Check)
	}
}

func TestDetectSimpleChangelog(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# Changelog\n\n## 0.1.0 2026-01-01\n"), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChangelogFormat != "simple" {
		t.Errorf("ChangelogFormat = %q, want simple", cfg.ChangelogFormat)
	}
}

func TestInstallNil(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Install != nil {
		t.Error("Install should be nil by default")
	}
	if cfg.ShouldInstall() {
		t.Error("ShouldInstall should return false when nil")
	}
}

func TestInstallFromYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("install: true\n"), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Install == nil {
		t.Fatal("Install should not be nil")
	}
	if !cfg.ShouldInstall() {
		t.Error("ShouldInstall should return true")
	}
}

func TestLoadYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `type: binary
check: [make test]
changelog_format: simple
cleanup:
  keep_patches: 3
  keep_minors: 10
`
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte(yaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Type != "binary" {
		t.Errorf("Type = %q, want binary", cfg.Type)
	}
	if len(cfg.Check) != 1 || cfg.Check[0] != "make test" {
		t.Errorf("Check = %v, want [make test]", cfg.Check)
	}
	if cfg.ChangelogFormat != "simple" {
		t.Errorf("ChangelogFormat = %q, want simple", cfg.ChangelogFormat)
	}
	if cfg.Cleanup.KeepPatches != 3 {
		t.Errorf("KeepPatches = %d, want 3", cfg.Cleanup.KeepPatches)
	}
}
