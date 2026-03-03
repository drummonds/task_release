package main

import "testing"

func TestHasTaskfileTask(t *testing.T) {
	taskfile := []byte(`version: "3"

tasks:
  fmt:
    cmds:
      - go fmt ./...
  release:
    cmds:
      - echo releasing
  pages:
    cmds:
      - echo pages
  build:docs:
    cmds:
      - echo build docs
  release:post:
    cmds:
      - echo release post
  post:release:
    cmds:
      - echo post release
`)

	tests := []struct {
		name string
		task string
		want bool
	}{
		{"release exists", "release", true},
		{"pages exists", "pages", true},
		{"fmt exists", "fmt", true},
		{"build:docs exists", "build:docs", true},
		{"nonexistent", "deploy", false},
		{"partial match", "rel", false},
		{"release:post exists", "release:post", true},
		{"post:release exists", "post:release", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTaskfileTask(taskfile, tt.task)
			if got != tt.want {
				t.Errorf("hasTaskfileTask(%q) = %v, want %v", tt.task, got, tt.want)
			}
		})
	}
}

func TestHasTaskfileTaskNoTasks(t *testing.T) {
	data := []byte(`version: "3"`)
	if hasTaskfileTask(data, "release") {
		t.Error("expected false for empty taskfile")
	}
}

func TestHasTaskfileTaskNamespacedOnly(t *testing.T) {
	// Only release:post exists, no bare release — should not match "release"
	data := []byte(`version: "3"

tasks:
  release:post:
    cmds:
      - echo post
  post:release:
    cmds:
      - echo pre
`)
	if hasTaskfileTask(data, "release") {
		t.Error("release:post should not match bare 'release'")
	}
	if !hasTaskfileTask(data, "release:post") {
		t.Error("expected release:post to match")
	}
	if !hasTaskfileTask(data, "post:release") {
		t.Error("expected post:release to match")
	}
}
