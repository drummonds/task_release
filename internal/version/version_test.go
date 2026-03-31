package version

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		input   string
		want    Version
		wantErr bool
	}{
		{"v1.2.3", Version{Major: 1, Minor: 2, Patch: 3}, false},
		{"v0.1.0", Version{Major: 0, Minor: 1, Patch: 0}, false},
		{"v10.20.30", Version{Major: 10, Minor: 20, Patch: 30}, false},
		{"v1.2.3-beta", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta"}, false},
		{"v0.2.0-mybranch.3", Version{Major: 0, Minor: 2, Patch: 0, Prerelease: "mybranch.3"}, false},
		{"1.2.3", Version{}, true},
		{"v1.2", Version{}, true},
		{"", Version{}, true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBumpPatch(t *testing.T) {
	v := Version{Major: 0, Minor: 1, Patch: 5}
	got := v.BumpPatch()
	want := Version{Major: 0, Minor: 1, Patch: 6}
	if got != want {
		t.Errorf("BumpPatch() = %v, want %v", got, want)
	}
}

func TestLatestFromTags(t *testing.T) {
	tags := []string{"v0.1.0", "v0.2.0", "v0.1.5", "not-a-tag", "v0.3.0"}
	got, found := LatestFromTags(tags)
	if !found {
		t.Fatal("expected to find a version")
	}
	want := Version{Major: 0, Minor: 3, Patch: 0}
	if got != want {
		t.Errorf("LatestFromTags() = %v, want %v", got, want)
	}
}

func TestLatestFromTagsIgnoresPrerelease(t *testing.T) {
	tags := []string{"v0.1.0", "v0.2.0-beta.1", "v0.1.5"}
	got, found := LatestFromTags(tags)
	if !found {
		t.Fatal("expected to find a version")
	}
	want := Version{Major: 0, Minor: 1, Patch: 5}
	if got != want {
		t.Errorf("LatestFromTags() = %v, want %v", got, want)
	}
}

func TestLatestFromTagsEmpty(t *testing.T) {
	_, found := LatestFromTags(nil)
	if found {
		t.Error("expected not found for nil tags")
	}
}

func TestString(t *testing.T) {
	v := Version{Major: 1, Minor: 2, Patch: 3}
	if got := v.String(); got != "v1.2.3" {
		t.Errorf("String() = %q, want %q", got, "v1.2.3")
	}
	if got := v.TagString(); got != "1.2.3" {
		t.Errorf("TagString() = %q, want %q", got, "1.2.3")
	}
}

func TestStringPrerelease(t *testing.T) {
	v := Version{Major: 0, Minor: 2, Patch: 0, Prerelease: "mybranch.1"}
	if got := v.String(); got != "v0.2.0-mybranch.1" {
		t.Errorf("String() = %q, want %q", got, "v0.2.0-mybranch.1")
	}
	if got := v.TagString(); got != "0.2.0-mybranch.1" {
		t.Errorf("TagString() = %q, want %q", got, "0.2.0-mybranch.1")
	}
}

func TestLessPrerelease(t *testing.T) {
	release := Version{Major: 1, Minor: 0, Patch: 0}
	prerelease := Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "beta.1"}

	if !prerelease.Less(release) {
		t.Error("pre-release should be less than release")
	}
	if release.Less(prerelease) {
		t.Error("release should not be less than pre-release")
	}
}

func TestParsePrerelease(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantIter int
	}{
		{"mybranch.3", "mybranch", 3},
		{"fix-foo.1", "fix-foo", 1},
		{"beta", "beta", 0},
		{"my.branch.2", "my.branch", 2},
	}
	for _, tt := range tests {
		name, iter := ParsePrerelease(tt.input)
		if name != tt.wantName || iter != tt.wantIter {
			t.Errorf("ParsePrerelease(%q) = (%q, %d), want (%q, %d)", tt.input, name, iter, tt.wantName, tt.wantIter)
		}
	}
}

func TestLatestPrereleaseFromTags(t *testing.T) {
	tags := []string{"v0.2.0-mybranch.1", "v0.2.0-mybranch.2", "v0.2.0-other.1", "v0.3.0"}
	base := Version{Major: 0, Minor: 2, Patch: 0}

	got, found := LatestPrereleaseFromTags(tags, base, "mybranch")
	if !found {
		t.Fatal("expected to find a pre-release version")
	}
	want := Version{Major: 0, Minor: 2, Patch: 0, Prerelease: "mybranch.2"}
	if got != want {
		t.Errorf("LatestPrereleaseFromTags() = %v, want %v", got, want)
	}
}

func TestLatestPrereleaseFromTagsNotFound(t *testing.T) {
	tags := []string{"v0.2.0", "v0.3.0"}
	base := Version{Major: 0, Minor: 2, Patch: 0}
	_, found := LatestPrereleaseFromTags(tags, base, "mybranch")
	if found {
		t.Error("expected not found")
	}
}

func TestBumpPrerelease(t *testing.T) {
	base := Version{Major: 0, Minor: 2, Patch: 0}
	tags := []string{"v0.2.0-mybranch.1", "v0.2.0-mybranch.2"}

	got := base.BumpPrerelease("mybranch", tags)
	want := Version{Major: 0, Minor: 2, Patch: 0, Prerelease: "mybranch.3"}
	if got != want {
		t.Errorf("BumpPrerelease() = %v, want %v", got, want)
	}
}

func TestBumpPrereleaseFirst(t *testing.T) {
	base := Version{Major: 0, Minor: 2, Patch: 0}
	got := base.BumpPrerelease("mybranch", nil)
	want := Version{Major: 0, Minor: 2, Patch: 0, Prerelease: "mybranch.1"}
	if got != want {
		t.Errorf("BumpPrerelease() = %v, want %v", got, want)
	}
}

func TestWithPrerelease(t *testing.T) {
	v := Version{Major: 1, Minor: 0, Patch: 0}
	got := v.WithPrerelease("beta.1")
	want := Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "beta.1"}
	if got != want {
		t.Errorf("WithPrerelease() = %v, want %v", got, want)
	}
}

func TestParseRC(t *testing.T) {
	tests := []struct {
		input  string
		wantN  int
		wantOK bool
	}{
		{"rc1", 1, true},
		{"rc3", 3, true},
		{"rc10", 10, true},
		{"beta", 0, false},
		{"rc", 0, false},
		{"rc0", 0, false},
		{"mybranch.1", 0, false},
	}
	for _, tt := range tests {
		n, ok := ParseRC(tt.input)
		if n != tt.wantN || ok != tt.wantOK {
			t.Errorf("ParseRC(%q) = (%d, %v), want (%d, %v)", tt.input, n, ok, tt.wantN, tt.wantOK)
		}
	}
}

func TestIsRC(t *testing.T) {
	rc := Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "rc1"}
	if !rc.IsRC() {
		t.Error("expected rc1 to be RC")
	}
	notRC := Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "beta"}
	if notRC.IsRC() {
		t.Error("expected beta to not be RC")
	}
	release := Version{Major: 1, Minor: 0, Patch: 0}
	if release.IsRC() {
		t.Error("expected release to not be RC")
	}
}

func TestLatestRCFromTags(t *testing.T) {
	tags := []string{"v1.0.0-rc1", "v1.0.0-rc2", "v1.0.0-rc3", "v1.0.0-beta.1", "v1.1.0-rc1"}
	base := Version{Major: 1, Minor: 0, Patch: 0}

	got, found := LatestRCFromTags(tags, base)
	if !found {
		t.Fatal("expected to find an RC version")
	}
	want := Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "rc3"}
	if got != want {
		t.Errorf("LatestRCFromTags() = %v, want %v", got, want)
	}
}

func TestLatestRCFromTagsNotFound(t *testing.T) {
	tags := []string{"v1.0.0", "v1.0.0-beta.1"}
	base := Version{Major: 1, Minor: 0, Patch: 0}
	_, found := LatestRCFromTags(tags, base)
	if found {
		t.Error("expected not found")
	}
}

func TestBumpRC(t *testing.T) {
	base := Version{Major: 1, Minor: 0, Patch: 0}
	tags := []string{"v1.0.0-rc1", "v1.0.0-rc2"}

	got := base.BumpRC(tags)
	want := Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "rc3"}
	if got != want {
		t.Errorf("BumpRC() = %v, want %v", got, want)
	}
}

func TestBumpRCFirst(t *testing.T) {
	base := Version{Major: 1, Minor: 0, Patch: 0}
	got := base.BumpRC(nil)
	want := Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "rc1"}
	if got != want {
		t.Errorf("BumpRC() = %v, want %v", got, want)
	}
}

func TestBase(t *testing.T) {
	v := Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "beta.1"}
	got := v.Base()
	want := Version{Major: 1, Minor: 0, Patch: 0}
	if got != want {
		t.Errorf("Base() = %v, want %v", got, want)
	}
}
