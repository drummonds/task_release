package check

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	findings := checkConfig(dir)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].level != levelWarn {
		t.Errorf("expected WARN, got %v", findings[0].level)
	}
}

func TestCheckConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte(":\n  :\n[bad"), 0644)
	findings := checkConfig(dir)
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
	if findings[0].level != levelError {
		t.Errorf("expected ERROR, got %v", findings[0].level)
	}
}

func TestCheckConfig_UnknownField(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("bogus_field: true\n"), 0644)
	findings := checkConfig(dir)
	hasUnknown := false
	for _, f := range findings {
		if f.level == levelWarn && f.message == `Unknown field "bogus_field"` {
			hasUnknown = true
		}
	}
	if !hasUnknown {
		t.Errorf("expected unknown field warning, got %v", findings)
	}
}

func TestCheckConfig_InvalidType(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("type: invalid\n"), 0644)
	findings := checkConfig(dir)
	hasError := false
	for _, f := range findings {
		if f.level == levelError && f.message == `Invalid type "invalid" (expected: binary, library, docs)` {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected invalid type error, got %v", findings)
	}
}

func TestCheckConfig_ChangemeWarning(t *testing.T) {
	dir := t.TempDir()
	content := "pages_deploy:\n  - type: statichost\n    site: CHANGEME\n"
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte(content), 0644)
	findings := checkConfig(dir)
	hasWarn := false
	for _, f := range findings {
		if f.level == levelWarn && f.message == "pages_deploy[0]: site is still 'CHANGEME'" {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected CHANGEME warning, got %v", findings)
	}
}

func TestCheckConfig_LanguagesDetected(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("remotes: [origin]\n"), 0644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0644)
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"test\"\n"), 0644)
	findings := checkConfig(dir)
	hasLangs := false
	for _, f := range findings {
		if f.level == levelOK && f.message == "Languages: go, python" {
			hasLangs = true
		}
	}
	if !hasLangs {
		t.Errorf("expected languages OK, got %v", findings)
	}
}

func TestCheckConfig_GoWithoutGoMod(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("languages: [go]\n"), 0644)
	findings := checkConfig(dir)
	hasError := false
	for _, f := range findings {
		if f.level == levelError && f.message == "Language 'go' but no go.mod found" {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected go/go.mod error, got %v", findings)
	}
}

func TestCheckConfig_PythonWithoutPyproject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("languages: [python]\n"), 0644)
	findings := checkConfig(dir)
	hasError := false
	for _, f := range findings {
		if f.level == levelError && f.message == "Language 'python' but no pyproject.toml found" {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected python/pyproject.toml error, got %v", findings)
	}
}

func TestCheckConfig_UnknownLanguage(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "task-plus.yml"), []byte("languages: [rust]\n"), 0644)
	findings := checkConfig(dir)
	hasWarn := false
	for _, f := range findings {
		if f.level == levelWarn && f.message == `Unknown language "rust" (expected: go, python)` {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected unknown language warning, got %v", findings)
	}
}

func TestCheckTaskfile_Missing(t *testing.T) {
	dir := t.TempDir()
	findings := checkTaskfile(dir)
	if len(findings) != 1 || findings[0].level != levelWarn {
		t.Errorf("expected 1 WARN, got %v", findings)
	}
}

func TestCheckTaskfile_StandardTasks(t *testing.T) {
	dir := t.TempDir()
	content := "version: '3'\ntasks:\n  fmt:\n    cmds: [echo]\n  vet:\n    cmds: [echo]\n  test:\n    cmds: [echo]\n  check:\n    cmds: [echo]\n"
	os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(content), 0644)
	findings := checkTaskfile(dir)
	hasOK := false
	for _, f := range findings {
		if f.level == levelOK && f.message == "Standard tasks: fmt, vet, test, check" {
			hasOK = true
		}
	}
	if !hasOK {
		t.Errorf("expected standard tasks OK, got %v", findings)
	}
}

func TestCheckTaskfile_MissingStandardTasks(t *testing.T) {
	dir := t.TempDir()
	content := "version: '3'\ntasks:\n  fmt:\n    cmds: [echo]\n"
	os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(content), 0644)
	findings := checkTaskfile(dir)
	hasWarn := false
	for _, f := range findings {
		if f.level == levelWarn && f.message == "Missing standard tasks: vet, test, check" {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Errorf("expected missing tasks warning, got %v", findings)
	}
}

func TestCheckTaskfile_Conflict(t *testing.T) {
	dir := t.TempDir()
	content := "version: '3'\ntasks:\n  release:\n    cmds: [echo]\n"
	os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(content), 0644)
	findings := checkTaskfile(dir)
	hasError := false
	for _, f := range findings {
		if f.level == levelError && f.message == `Task "release" conflicts with 'tp release' command — remove it` {
			hasError = true
		}
	}
	if !hasError {
		t.Errorf("expected conflict error, got %v", findings)
	}
}

func TestCheckTaskfile_Inversion(t *testing.T) {
	dir := t.TempDir()
	content := "version: '3'\ntasks:\n  build:docs:\n    cmds: [echo]\n  tidy:deps:\n    cmds: [echo]\n"
	os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(content), 0644)
	findings := checkTaskfile(dir)

	inversions := 0
	for _, f := range findings {
		if f.level == levelWarn {
			switch f.message {
			case "Has 'build:docs' — rename to 'docs:build' (subject:action convention)":
				inversions++
			case "Has 'tidy:deps' — rename to 'deps:tidy' (subject:action convention)":
				inversions++
			}
		}
	}
	if inversions != 2 {
		t.Errorf("expected 2 inversion warnings, got %d from %v", inversions, findings)
	}
}

func TestCheckTaskfile_NoInversionWhenPreferredExists(t *testing.T) {
	dir := t.TempDir()
	// Both preferred and inverted exist — no warning (user may have aliases)
	content := "version: '3'\ntasks:\n  docs:build:\n    cmds: [echo]\n  build:docs:\n    cmds: [echo]\n"
	os.WriteFile(filepath.Join(dir, "Taskfile.yml"), []byte(content), 0644)
	findings := checkTaskfile(dir)
	for _, f := range findings {
		if f.level == levelWarn && f.message == "Has 'build:docs' — rename to 'docs:build' (subject:action convention)" {
			t.Errorf("should not warn when preferred form also exists")
		}
	}
}
