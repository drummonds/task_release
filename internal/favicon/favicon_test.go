package favicon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitials(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"task-plus", "TP"},
		{"lofigui", "LO"},
		{"go-postgres", "PO"},
		{"my_project", "MP"},
		{"simple", "SI"},
		{"a", "A"},
		{"a-b-c", "AB"},
		{"", "?"},
	}
	for _, tt := range tests {
		got := Initials(tt.name)
		if got != tt.want {
			t.Errorf("Initials(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(dir, "TP", "#3273dc"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "favicon.svg")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svg := string(data)
	if len(svg) == 0 {
		t.Fatal("empty favicon.svg")
	}
	if !contains(svg, "TP") {
		t.Error("SVG missing text 'TP'")
	}
	if !contains(svg, "#3273dc") {
		t.Error("SVG missing color")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	if Exists(dir) {
		t.Error("Exists should be false for empty dir")
	}
	if err := os.WriteFile(filepath.Join(dir, "favicon.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir) {
		t.Error("Exists should be true after creating favicon.svg")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
