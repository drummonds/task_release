package git

import "testing"

func TestURLToWeb(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ssh codeberg", "ssh://git@codeberg.org/hum3/task-plus.git", "https://codeberg.org/hum3/task-plus"},
		{"ssh no .git", "ssh://git@codeberg.org/hum3/task-plus", "https://codeberg.org/hum3/task-plus"},
		{"scp github", "git@github.com:drummonds/task-plus.git", "https://github.com/drummonds/task-plus"},
		{"scp no .git", "git@github.com:drummonds/task-plus", "https://github.com/drummonds/task-plus"},
		{"https", "https://codeberg.org/hum3/task-plus", "https://codeberg.org/hum3/task-plus"},
		{"https .git", "https://codeberg.org/hum3/task-plus.git", "https://codeberg.org/hum3/task-plus"},
		{"http", "http://example.com/repo", "http://example.com/repo"},
		{"empty", "", ""},
		{"unknown", "file:///local/repo", ""},
		{"whitespace", "  ssh://git@codeberg.org/hum3/repo.git  ", "https://codeberg.org/hum3/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := URLToWeb(tt.in)
			if got != tt.want {
				t.Errorf("URLToWeb(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
