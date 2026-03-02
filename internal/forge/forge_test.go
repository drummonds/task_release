package forge

import "testing"

func TestDetectFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want Type
	}{
		// GitHub
		{"https://github.com/user/repo.git", GitHub},
		{"git@github.com:user/repo.git", GitHub},

		// GitLab
		{"https://gitlab.com/user/repo.git", GitLab},
		{"git@gitlab.com:user/repo.git", GitLab},
		{"https://gitlab.example.com/user/repo.git", GitLab},

		// Forgejo / Codeberg
		{"https://codeberg.org/user/repo.git", Forgejo},
		{"git@codeberg.org:user/repo.git", Forgejo},
		{"https://gitea.example.com/user/repo.git", Forgejo},
		{"https://forgejo.example.com/user/repo.git", Forgejo},

		// Unknown
		{"https://bitbucket.org/user/repo.git", Unknown},
		{"git@unknown.host:user/repo.git", Unknown},
	}

	for _, tt := range tests {
		got := detectFromURL(tt.url)
		if got != tt.want {
			t.Errorf("detectFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"git@github.com:user/repo.git", "github.com"},
		{"https://github.com/user/repo.git", "github.com"},
		{"https://gitlab.example.com/user/repo.git", "gitlab.example.com"},
		{"git@gitlab.example.com:user/repo.git", "gitlab.example.com"},
	}

	for _, tt := range tests {
		got := extractHost(tt.url)
		if got != tt.want {
			t.Errorf("extractHost(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestParseGLabReleaseList(t *testing.T) {
	output := `v0.3.0    Release 0.3.0    2024-01-15T10:00:00Z
v0.2.1    Bug fix release  2024-01-10T10:00:00Z
v0.2.0    Initial release  2024-01-01T10:00:00Z
some-tag  Not a version    2023-12-01T10:00:00Z
`
	got := parseGLabReleaseList(output)
	want := []string{"v0.3.0", "v0.2.1", "v0.2.0"}

	if len(got) != len(want) {
		t.Fatalf("parseGLabReleaseList got %d tags, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("tag[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
