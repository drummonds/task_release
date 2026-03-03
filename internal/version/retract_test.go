package version

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRetracted(t *testing.T) {
	dir := t.TempDir()
	gomod := `module example.com/foo

go 1.25.0

retract (
	v1.37.6 // Contains only retractions
	v1.37.5 // Contains only retractions
	v1.37.4 // Accidentally published
)
`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644)

	got, err := ParseRetracted(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{{1, 37, 6}, {1, 37, 5}, {1, 37, 4}}
	if len(got) != len(want) {
		t.Fatalf("got %d retracted, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("retracted[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestParseRetractedSingleLine(t *testing.T) {
	dir := t.TempDir()
	gomod := `module example.com/foo

go 1.25.0

retract v0.1.9 // broken
`
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0644)

	got, err := ParseRetracted(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != (Version{0, 1, 9}) {
		t.Errorf("got %v, want [v0.1.9]", got)
	}
}

func TestParseRetractedNoGoMod(t *testing.T) {
	dir := t.TempDir()
	got, err := ParseRetracted(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestLatestFromTagsWithRetracted(t *testing.T) {
	tags := []string{"v0.48.12", "v1.37.4", "v1.37.5", "v1.37.6"}
	retracted := []Version{{1, 37, 4}, {1, 37, 5}, {1, 37, 6}}

	got, found := LatestFromTags(tags, retracted)
	if !found {
		t.Fatal("expected to find a version")
	}
	want := Version{0, 48, 12}
	if got != want {
		t.Errorf("LatestFromTags() = %v, want %v", got, want)
	}
}

func TestBumpPastRetracted(t *testing.T) {
	v := Version{0, 1, 8}
	retracted := []Version{{0, 1, 9}, {0, 1, 10}}
	got := v.BumpPastRetracted(retracted)
	want := Version{0, 1, 11}
	if got != want {
		t.Errorf("BumpPastRetracted() = %v, want %v", got, want)
	}
}

func TestBumpPastRetractedNone(t *testing.T) {
	v := Version{0, 1, 8}
	got := v.BumpPastRetracted(nil)
	want := Version{0, 1, 9}
	if got != want {
		t.Errorf("BumpPastRetracted() = %v, want %v", got, want)
	}
}

func TestParseGitURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"git@github.com:drummonds/task-plus.git", "github.com/drummonds/task-plus"},
		{"git@github.com:user/repo.git", "github.com/user/repo"},
		{"git@gitlab.com:org/project.git", "gitlab.com/org/project"},
		{"git@github.com:user/repo", "github.com/user/repo"},
		{"https://github.com/drummonds/task-plus.git", "github.com/drummonds/task-plus"},
		{"https://github.com/user/repo", "github.com/user/repo"},
		{"http://github.com/user/repo.git", "github.com/user/repo"},
		{"", ""},
		{"file:///local/repo", ""},
	}
	for _, tt := range tests {
		got := ParseGitURL(tt.input)
		if got != tt.want {
			t.Errorf("ParseGitURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
