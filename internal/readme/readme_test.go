package readme

import (
	"testing"

	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/deploy"
)

func TestReplaceSection(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		section     string
		replacement string
		want        string
		wantOK      bool
	}{
		{
			name:        "simple replace",
			content:     "before <!-- auto:version -->old<!-- /auto:version --> after",
			section:     "version",
			replacement: "new",
			want:        "before <!-- auto:version -->new<!-- /auto:version --> after",
			wantOK:      true,
		},
		{
			name:        "multiline replace",
			content:     "# Links\n\n<!-- auto:links -->\nold table\n<!-- /auto:links -->\n",
			section:     "links",
			replacement: "\nnew table\n",
			want:        "# Links\n\n<!-- auto:links -->\nnew table\n<!-- /auto:links -->\n",
			wantOK:      true,
		},
		{
			name:        "no markers",
			content:     "plain content",
			section:     "version",
			replacement: "v1.0.0",
			want:        "plain content",
			wantOK:      false,
		},
		{
			name:        "open without close",
			content:     "<!-- auto:version -->orphaned",
			section:     "version",
			replacement: "v1.0.0",
			want:        "<!-- auto:version -->orphaned",
			wantOK:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ReplaceSection(tt.content, tt.section, tt.replacement)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestDocsLinks(t *testing.T) {
	tests := []struct {
		name    string
		targets []deploy.Target
		want    []docLink
	}{
		{
			name:    "no targets",
			targets: nil,
			want:    nil,
		},
		{
			name: "single target",
			targets: []deploy.Target{
				{Type: "statichost", Site: "h3-task-plus"},
			},
			want: []docLink{
				{"Documentation", "https://h3-task-plus.statichost.page/"},
			},
		},
		{
			name: "single target with RC",
			targets: []deploy.Target{
				{Type: "statichost", Site: "h3-gobank", RCSite: "h3-gobank-rc"},
			},
			want: []docLink{
				{"Documentation", "https://h3-gobank.statichost.page/"},
				{"RC Documentation", "https://h3-gobank-rc.statichost.page/"},
			},
		},
		{
			name: "multiple targets",
			targets: []deploy.Target{
				{Type: "statichost", Site: "h3-bytestoneblog"},
				{Type: "statichost", Site: "blog-bytestone"},
			},
			want: []docLink{
				{"Documentation", "https://h3-bytestoneblog.statichost.page/"},
				{"Blog Bytestone", "https://blog-bytestone.statichost.page/"},
			},
		},
		{
			name: "multiple targets with RC",
			targets: []deploy.Target{
				{Type: "statichost", Site: "h3-bytestoneblog", RCSite: "h3-bytestoneblog-rc"},
				{Type: "statichost", Site: "blog-bytestone"},
			},
			want: []docLink{
				{"Documentation", "https://h3-bytestoneblog.statichost.page/"},
				{"RC Documentation", "https://h3-bytestoneblog-rc.statichost.page/"},
				{"Blog Bytestone", "https://blog-bytestone.statichost.page/"},
			},
		},
		{
			name: "skips non-statichost",
			targets: []deploy.Target{
				{Type: "github"},
				{Type: "statichost", Site: "h3-foo"},
			},
			want: []docLink{
				{"Documentation", "https://h3-foo.statichost.page/"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{PagesDeploy: tt.targets}
			got := docsLinks(cfg)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d links, want %d: %v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("link[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSiteLabel(t *testing.T) {
	tests := []struct {
		site string
		want string
	}{
		{"h3-task-plus", "Task Plus"},
		{"blog-bytestone", "Blog Bytestone"},
		{"h3-docs", "Docs"},
		{"mysite", "Mysite"},
	}
	for _, tt := range tests {
		t.Run(tt.site, func(t *testing.T) {
			if got := siteLabel(tt.site); got != tt.want {
				t.Errorf("siteLabel(%q) = %q, want %q", tt.site, got, tt.want)
			}
		})
	}
}

func TestGenerateVersion(t *testing.T) {
	got := GenerateVersion("v0.1.46")
	want := "Latest: v0.1.46"
	if got != want {
		t.Errorf("GenerateVersion(v0.1.46) = %q, want %q", got, want)
	}
}
