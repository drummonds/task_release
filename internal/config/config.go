package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type CleanupConfig struct {
	KeepPatches int `yaml:"keep_patches"`
	KeepMinors  int `yaml:"keep_minors"`
}

type Config struct {
	Type             string        `yaml:"type"`
	Precheck         []string      `yaml:"precheck"`
	Check            []string      `yaml:"check"`
	ChangelogFormat  string        `yaml:"changelog_format"`
	Wasm             []string      `yaml:"wasm"`
	GoreleaserConfig string        `yaml:"goreleaser_config"`
	Forge            string        `yaml:"forge"`
	Cleanup          CleanupConfig `yaml:"cleanup"`
	Fork             *bool         `yaml:"fork"`
	Install          *bool         `yaml:"install"`
	InstallRetries   int           `yaml:"install_retries"`
	PagesBuild       []string      `yaml:"pages_build"`
	Dir              string        `yaml:"-"`
}

// ShouldInstall returns the configured install preference, or false if not set.
func (c *Config) ShouldInstall() bool {
	if c.Install == nil {
		return false
	}
	return *c.Install
}

const configFile = "task-plus.yml"

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
	if c.InstallRetries == 0 {
		c.InstallRetries = 3
	}
	if len(c.PagesBuild) == 0 {
		c.PagesBuild = c.detectPagesBuild()
	}
}

func (c *Config) detectType() string {
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
	return []string{"go fmt ./...", "go vet ./...", "go test ./..."}
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
// Looks for build:docs task. Warns if build-pages is found (should be renamed).
func (c *Config) detectPagesBuild() []string {
	data, err := os.ReadFile(filepath.Join(c.Dir, "Taskfile.yml"))
	if err != nil {
		return nil
	}
	lines := splitLines(string(data))
	inTasks := false
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
			if trimmed == "build:docs:" || (len(trimmed) > 11 && trimmed[:11] == "build:docs:") {
				hasBuildDocs = true
			}
			if trimmed == "build-pages:" || (len(trimmed) > 12 && trimmed[:12] == "build-pages:") {
				hasBuildPages = true
			}
		}
	}
	if hasBuildDocs {
		return []string{"task build:docs"}
	}
	if hasBuildPages {
		fmt.Fprintf(os.Stderr, "Warning: Taskfile has 'build-pages' task — consider renaming to 'build:docs' for auto-detection.\n")
	}
	return nil
}

// HasPagesBuild returns true if page build steps are configured.
func (c *Config) HasPagesBuild() bool {
	return len(c.PagesBuild) > 0
}

// IsBinary returns true if this is a binary (goreleaser) project.
func (c *Config) IsBinary() bool {
	return c.Type == "binary"
}

// HasWasm returns true if WASM build steps are configured.
func (c *Config) HasWasm() bool {
	return len(c.Wasm) > 0
}
