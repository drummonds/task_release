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
