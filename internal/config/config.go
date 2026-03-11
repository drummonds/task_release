package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/drummonds/task-plus/internal/deploy"
	"gopkg.in/yaml.v3"
)

type CleanupConfig struct {
	KeepPatches int `yaml:"keep_patches"`
	KeepMinors  int `yaml:"keep_minors"`
}

type Config struct {
	Type             string          `yaml:"type"`
	Languages        []string        `yaml:"languages"`
	Precheck         []string        `yaml:"precheck"`
	Check            []string        `yaml:"check"`
	ChangelogFormat  string          `yaml:"changelog_format"`
	Wasm             []string        `yaml:"wasm"`
	GoreleaserConfig string          `yaml:"goreleaser_config"`
	Forge            string          `yaml:"forge"`
	Remotes          []string        `yaml:"remotes"`
	Cleanup          CleanupConfig   `yaml:"cleanup"`
	Fork             *bool           `yaml:"fork"`
	Install          *bool           `yaml:"install"`
	InstallRetries   int             `yaml:"install_retries"`
	PagesBuild       []string        `yaml:"pages_build"`
	PagesDeploy      []deploy.Target `yaml:"pages_deploy"`
	DocsRepo         string          `yaml:"docs_repo"`
	ParentRepo       string          `yaml:"parent_repo"`
	Dir              string          `yaml:"-"`
}

// ShouldInstall returns the configured install preference, or false if not set.
func (c *Config) ShouldInstall() bool {
	if c.Install == nil {
		return false
	}
	return *c.Install
}

const configFile = "task-plus.yml"

// Init creates a default task-plus.yml in dir. Returns an error if the file already exists.
func Init(dir string) error {
	path := filepath.Join(dir, configFile)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", configFile)
	}

	content := `# task-plus configuration — see https://github.com/drummonds/task-plus
# type: library           # or "binary" (auto-detected from .goreleaser.yaml)
# check: [task check]     # commands to run during release checks
# changelog_format: keepachangelog  # or "simple"
# install: true           # auto-run "go install" after release
# remotes: [origin]       # git remotes to push to (default: origin)

pages_deploy:
  - type: statichost
    site: CHANGEME         # your site name on statichost.eu
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Printf("Created %s\n", configFile)
	return nil
}

// Load reads task-release.yml from dir, then applies auto-detection for unset fields.
func Load(dir string) (*Config, error) {
	c := &Config{Dir: dir}

	data, err := os.ReadFile(filepath.Join(dir, configFile))
	if err == nil {
		if err := yaml.Unmarshal(data, c); err != nil {
			return nil, err
		}
	}

	c.applyDefaults()
	return c, nil
}

func (c *Config) applyDefaults() {
	if c.Type == "" {
		c.Type = c.detectType()
	}
	if len(c.Languages) == 0 {
		c.Languages = c.detectLanguages()
	}
	if len(c.Precheck) == 0 {
		c.Precheck = c.detectPrecheck()
	}
	if len(c.Check) == 0 {
		c.Check = c.detectCheck()
	}
	if c.ChangelogFormat == "" {
		c.ChangelogFormat = c.detectChangelogFormat()
	}
	if c.GoreleaserConfig == "" {
		c.GoreleaserConfig = ".goreleaser.yaml"
	}
	if c.Cleanup.KeepPatches == 0 {
		c.Cleanup.KeepPatches = 2
	}
	if c.Cleanup.KeepMinors == 0 {
		c.Cleanup.KeepMinors = 5
	}
	if len(c.Remotes) == 0 {
		c.Remotes = []string{"origin"}
	}
	if c.InstallRetries == 0 {
		c.InstallRetries = 3
	}
	if len(c.PagesBuild) == 0 {
		c.PagesBuild = c.detectPagesBuild()
	}
}

func (c *Config) detectType() string {
	// If parent_repo is set, this is a docs project
	if c.ParentRepo != "" {
		return "docs"
	}
	name := c.detectGoreleaserConfig()
	if name != "" {
		c.GoreleaserConfig = name
		return "binary"
	}
	return "library"
}

// detectGoreleaserConfig returns the goreleaser config filename if found, or "".
func (c *Config) detectGoreleaserConfig() string {
	for _, name := range []string{".goreleaser.yaml", ".goreleaser.yml"} {
		if _, err := os.Stat(filepath.Join(c.Dir, name)); err == nil {
			return name
		}
	}
	return ""
}

func (c *Config) detectPrecheck() []string {
	data, err := os.ReadFile(filepath.Join(c.Dir, "Taskfile.yml"))
	if err != nil {
		return nil
	}
	if hasTask(data, "precheck") {
		return []string{"task precheck"}
	}
	return nil
}

func (c *Config) detectCheck() []string {
	if _, err := os.Stat(filepath.Join(c.Dir, "Taskfile.yml")); err == nil {
		return []string{"task check"}
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "go.mod")); err == nil {
		return []string{"go fmt ./...", "go vet ./...", "go test ./..."}
	}
	return nil
}

// hasTask checks if YAML data contains a top-level task with the given name.
// Matches "  release:" but not "  release:post:" (colon-namespaced tasks are distinct).
func hasTask(data []byte, taskName string) bool {
	prefix := "  " + taskName + ":"
	lines := splitLines(string(data))
	inTasks := false
	for _, line := range lines {
		if line == "tasks:" {
			inTasks = true
			continue
		}
		if inTasks && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inTasks = false
		}
		if inTasks && len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			if len(line) == len(prefix) || line[len(prefix)] == ' ' {
				return true
			}
		}
	}
	return false
}

// HasTaskfileTask checks if a Taskfile in dir contains a top-level task with the given name.
func HasTaskfileTask(dir string, taskName string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "Taskfile.yml"))
	if err != nil {
		return false
	}
	return hasTask(data, taskName)
}

func (c *Config) detectChangelogFormat() string {
	data, err := os.ReadFile(filepath.Join(c.Dir, "CHANGELOG.md"))
	if err != nil {
		return "keepachangelog"
	}
	// Simple format uses "## 0.x.x YYYY-MM-DD" (no brackets)
	// Keepachangelog uses "## [0.x.x] - YYYY-MM-DD"
	content := string(data)
	if len(content) > 0 {
		for _, line := range splitLines(content) {
			if len(line) > 3 && line[:3] == "## " {
				rest := line[3:]
				if len(rest) > 0 && rest[0] == '[' {
					return "keepachangelog"
				}
				if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
					return "simple"
				}
			}
		}
	}
	return "keepachangelog"
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// detectPagesBuild auto-detects a build command from Taskfile.yml.
// Preferred task name is docs:build (subject:action, consistent with deps:tidy).
// Also accepts build:docs with a rename suggestion.
func (c *Config) detectPagesBuild() []string {
	data, err := os.ReadFile(filepath.Join(c.Dir, "Taskfile.yml"))
	if err != nil {
		return nil
	}
	lines := splitLines(string(data))
	inTasks := false
	hasDocsBuild := false
	hasBuildDocs := false
	hasBuildPages := false
	for _, line := range lines {
		if line == "tasks:" {
			inTasks = true
			continue
		}
		if inTasks && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inTasks = false
		}
		if inTasks {
			trimmed := line
			for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
				trimmed = trimmed[1:]
			}
			if trimmed == "docs:build:" || (len(trimmed) > 11 && trimmed[:11] == "docs:build:") {
				hasDocsBuild = true
			}
			if trimmed == "build:docs:" || (len(trimmed) > 11 && trimmed[:11] == "build:docs:") {
				hasBuildDocs = true
			}
			if trimmed == "build-pages:" || (len(trimmed) > 12 && trimmed[:12] == "build-pages:") {
				hasBuildPages = true
			}
		}
	}
	if hasDocsBuild {
		return []string{"task docs:build"}
	}
	if hasBuildDocs {
		fmt.Fprintf(os.Stderr, "Warning: Taskfile has 'build:docs' task — consider renaming to 'docs:build' (subject:action convention, like deps:tidy).\n")
		return []string{"task build:docs"}
	}
	if hasBuildPages {
		fmt.Fprintf(os.Stderr, "Warning: Taskfile has 'build-pages' task — consider renaming to 'docs:build' for auto-detection.\n")
	}
	return nil
}

// HasGoMod returns true if a go.mod file exists in the project directory.
func (c *Config) HasGoMod() bool {
	_, err := os.Stat(filepath.Join(c.Dir, "go.mod"))
	return err == nil
}

// HasPyproject returns true if a pyproject.toml file exists in the project directory.
func (c *Config) HasPyproject() bool {
	_, err := os.Stat(filepath.Join(c.Dir, "pyproject.toml"))
	return err == nil
}

// HasGo returns true if "go" is in the detected languages.
func (c *Config) HasGo() bool {
	return slices.Contains(c.Languages, "go")
}

// HasPython returns true if "python" is in the detected languages.
func (c *Config) HasPython() bool {
	return slices.Contains(c.Languages, "python")
}

// detectLanguages auto-detects project languages from marker files.
func (c *Config) detectLanguages() []string {
	var langs []string
	if c.HasGoMod() {
		langs = append(langs, "go")
	}
	if c.HasPyproject() {
		langs = append(langs, "python")
	}
	return langs
}

// PypiPackageName reads the package name from pyproject.toml's [project] name field.
// Returns "" if not found.
func (c *Config) PypiPackageName() string {
	data, err := os.ReadFile(filepath.Join(c.Dir, "pyproject.toml"))
	if err != nil {
		return ""
	}
	// Simple line-based parser — avoids TOML dependency
	inProject := false
	for _, line := range splitLines(string(data)) {
		trimmed := line
		for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
			trimmed = trimmed[1:]
		}
		if trimmed == "[project]" {
			inProject = true
			continue
		}
		if len(trimmed) > 0 && trimmed[0] == '[' {
			inProject = false
			continue
		}
		if inProject && len(trimmed) > 7 && trimmed[:5] == "name " || inProject && len(trimmed) > 5 && trimmed[:5] == "name=" {
			// Extract value from: name = "foo" or name="foo"
			idx := 0
			for idx < len(trimmed) && trimmed[idx] != '=' {
				idx++
			}
			if idx >= len(trimmed) {
				continue
			}
			val := trimmed[idx+1:]
			// Trim spaces and quotes
			for len(val) > 0 && (val[0] == ' ' || val[0] == '\t') {
				val = val[1:]
			}
			if len(val) >= 2 && val[0] == '"' {
				end := 1
				for end < len(val) && val[end] != '"' {
					end++
				}
				return val[1:end]
			}
		}
	}
	return ""
}

// HasPagesBuild returns true if page build steps are configured.
func (c *Config) HasPagesBuild() bool {
	return len(c.PagesBuild) > 0
}

// IsBinary returns true if this is a binary (goreleaser) project.
func (c *Config) IsBinary() bool {
	return c.Type == "binary"
}

// IsDocs returns true if this is a documentation-only project.
func (c *Config) IsDocs() bool {
	return c.Type == "docs"
}

// HasWasm returns true if WASM build steps are configured.
func (c *Config) HasWasm() bool {
	return len(c.Wasm) > 0
}

// HasPagesDeploy returns true if any deploy targets are configured.
func (c *Config) HasPagesDeploy() bool {
	return len(c.PagesDeploy) > 0
}

// PrimaryRemote returns the first configured remote name.
func (c *Config) PrimaryRemote() string {
	if len(c.Remotes) == 0 {
		return "origin"
	}
	return c.Remotes[0]
}
