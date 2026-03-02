package forge

import (
	"fmt"
	"os/exec"
	"strings"
)

// Type identifies a git forge provider.
type Type string

const (
	GitHub  Type = "github"
	GitLab  Type = "gitlab"
	Forgejo Type = "forgejo"
	Unknown Type = "unknown"
)

// Forge holds the detected forge type for a repository.
type Forge struct {
	Type Type
}

// Detect determines the forge from a config override or the git remote URL.
func Detect(dir, override string) (Forge, error) {
	if override != "" {
		return Forge{Type: Type(override)}, nil
	}
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Forge{Type: Unknown}, nil
	}
	url := strings.TrimSpace(string(out))
	return Forge{Type: detectFromURL(url)}, nil
}

// extractHost returns the hostname from an SSH or HTTPS git URL.
func extractHost(url string) string {
	// SSH: git@host:path
	if strings.HasPrefix(url, "git@") {
		rest := url[4:]
		if host, _, ok := strings.Cut(rest, ":"); ok {
			return host
		}
		return rest
	}
	// HTTPS: https://host/path
	if _, rest, ok := strings.Cut(url, "://"); ok {
		if i := strings.IndexAny(rest, ":/"); i >= 0 {
			return rest[:i]
		}
		return rest
	}
	return url
}

// detectFromURL maps a git remote URL to a forge type.
func detectFromURL(url string) Type {
	host := strings.ToLower(extractHost(url))
	switch {
	case host == "github.com":
		return GitHub
	case host == "gitlab.com" || strings.Contains(host, "gitlab"):
		return GitLab
	case host == "codeberg.org" || strings.Contains(host, "gitea") || strings.Contains(host, "forgejo"):
		return Forgejo
	default:
		return Unknown
	}
}

// HasCLI returns true if the appropriate CLI tool is available in PATH.
func (f Forge) HasCLI() bool {
	switch f.Type {
	case GitHub:
		_, err := exec.LookPath("gh")
		return err == nil
	case GitLab:
		_, err := exec.LookPath("glab")
		return err == nil
	case Forgejo:
		// TODO: forgejo-cli (fj) lacks release list/delete commands.
		// Revisit when CLI support matures.
		return false
	default:
		return false
	}
}

// ListReleases returns release tag names from the forge.
func (f Forge) ListReleases(dir string) ([]string, error) {
	switch f.Type {
	case GitHub:
		return listReleasesGitHub(dir)
	case GitLab:
		return listReleasesGitLab(dir)
	case Forgejo:
		return nil, fmt.Errorf("forgejo release listing not yet supported")
	default:
		return nil, fmt.Errorf("unknown forge type %q", f.Type)
	}
}

// DeleteRelease deletes a release by tag on the forge.
func (f Forge) DeleteRelease(dir, tag string) error {
	switch f.Type {
	case GitHub:
		return deleteReleaseGitHub(dir, tag)
	case GitLab:
		return deleteReleaseGitLab(dir, tag)
	case Forgejo:
		return fmt.Errorf("forgejo release deletion not yet supported")
	default:
		return fmt.Errorf("unknown forge type %q", f.Type)
	}
}

func listReleasesGitHub(dir string) ([]string, error) {
	cmd := exec.Command("gh", "release", "list", "--limit", "100", "--json", "tagName", "-q", ".[].tagName")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh release list: %w\n%s", err, out)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}

func deleteReleaseGitHub(dir, tag string) error {
	cmd := exec.Command("gh", "release", "delete", tag, "--yes", "--cleanup-tag")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh release delete %s: %w\n%s", tag, err, out)
	}
	return nil
}

func listReleasesGitLab(dir string) ([]string, error) {
	cmd := exec.Command("glab", "release", "list", "--per-page", "100")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("glab release list: %w\n%s", err, out)
	}
	return parseGLabReleaseList(string(out)), nil
}

// parseGLabReleaseList extracts version tags from glab release list output.
// Each line's first whitespace-delimited field is checked for a leading "v".
func parseGLabReleaseList(output string) []string {
	var tags []string
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.HasPrefix(fields[0], "v") {
			tags = append(tags, fields[0])
		}
	}
	return tags
}

func deleteReleaseGitLab(dir, tag string) error {
	cmd := exec.Command("glab", "release", "delete", tag, "-y", "--with-tag")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("glab release delete %s: %w\n%s", tag, err, out)
	}
	return nil
}
