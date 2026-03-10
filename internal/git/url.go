package git

import "strings"

// URLToWeb converts a git remote URL to a web-browsable URL.
// Handles ssh:// URLs, SCP-style (git@host:org/repo), and HTTPS formats.
func URLToWeb(rawURL string) string {
	u := strings.TrimSpace(rawURL)
	u = strings.TrimSuffix(u, ".git")

	// SSH URL: ssh://git@host/org/repo
	if strings.HasPrefix(u, "ssh://git@") {
		u = strings.TrimPrefix(u, "ssh://git@")
		return "https://" + u
	}
	// SCP-style: git@host:org/repo -> https://host/org/repo
	if strings.HasPrefix(u, "git@") {
		u = strings.TrimPrefix(u, "git@")
		u = strings.Replace(u, ":", "/", 1)
		return "https://" + u
	}
	// Already HTTPS/HTTP
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		return u
	}
	return ""
}
