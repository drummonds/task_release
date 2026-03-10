package readme

import "testing"

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

func TestGenerateVersion(t *testing.T) {
	got := GenerateVersion("v0.1.46")
	want := "Latest: v0.1.46"
	if got != want {
		t.Errorf("GenerateVersion(v0.1.46) = %q, want %q", got, want)
	}
}
