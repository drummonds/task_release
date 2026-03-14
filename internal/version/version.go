package version

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var semverRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:-(.+))?$`)

type Version struct {
	Major, Minor, Patch int
	Prerelease          string
}

func (v Version) String() string {
	s := fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	return s
}

// TagString returns the version without 'v' prefix (for changelogs).
func (v Version) TagString() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	return s
}

// Base returns the version without pre-release.
func (v Version) Base() Version {
	return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
}

// WithPrerelease returns a copy with the given pre-release string.
func (v Version) WithPrerelease(pre string) Version {
	return Version{v.Major, v.Minor, v.Patch, pre}
}

// Parse parses a "vX.Y.Z" or "vX.Y.Z-prerelease" string.
func Parse(s string) (Version, error) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("invalid version: %q", s)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return Version{Major: major, Minor: minor, Patch: patch, Prerelease: m[4]}, nil
}

// BumpPatch returns a new version with patch incremented (drops pre-release).
func (v Version) BumpPatch() Version {
	return Version{v.Major, v.Minor, v.Patch + 1, ""}
}

// Less returns true if v < other (per semver: pre-release < release for same base).
func (v Version) Less(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	if v.Patch != other.Patch {
		return v.Patch < other.Patch
	}
	// Same base version: pre-release sorts before release
	if v.Prerelease == "" && other.Prerelease == "" {
		return false
	}
	if v.Prerelease == "" {
		return false // release > pre-release
	}
	if other.Prerelease == "" {
		return true // pre-release < release
	}
	return v.Prerelease < other.Prerelease
}

// LatestFromTags finds the latest non-retracted semver tag from a list of tag strings.
// Pre-release tags are ignored — use LatestPrereleaseFromTags for those.
func LatestFromTags(tags []string, retracted ...[]Version) (Version, bool) {
	var exclude []Version
	if len(retracted) > 0 {
		exclude = retracted[0]
	}
	var versions []Version
	for _, t := range tags {
		t = strings.TrimSpace(t)
		v, err := Parse(t)
		if err == nil && v.Prerelease == "" && !IsRetracted(v, exclude) {
			versions = append(versions, v)
		}
	}
	if len(versions) == 0 {
		return Version{}, false
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Less(versions[j])
	})
	return versions[len(versions)-1], true
}

// ParsePrerelease splits a pre-release string like "branchname.3" into (name, iteration).
// Returns (pre, 0) if no numeric suffix found.
func ParsePrerelease(pre string) (string, int) {
	i := strings.LastIndex(pre, ".")
	if i < 0 {
		return pre, 0
	}
	n, err := strconv.Atoi(pre[i+1:])
	if err != nil {
		return pre, 0
	}
	return pre[:i], n
}

// LatestPrereleaseFromTags finds the highest iteration for a given base version + pre-release name.
func LatestPrereleaseFromTags(tags []string, base Version, name string) (Version, bool) {
	var best Version
	found := false
	for _, t := range tags {
		t = strings.TrimSpace(t)
		v, err := Parse(t)
		if err != nil || v.Prerelease == "" {
			continue
		}
		if v.Major != base.Major || v.Minor != base.Minor || v.Patch != base.Patch {
			continue
		}
		preName, _ := ParsePrerelease(v.Prerelease)
		if preName != name {
			continue
		}
		if !found || !v.Less(best) {
			best = v
			found = true
		}
	}
	return best, found
}

// BumpPrerelease returns the next pre-release iteration for the given name and base version.
// If no existing pre-release tags match, returns base-name.1.
func (v Version) BumpPrerelease(name string, tags []string) Version {
	base := v.Base()
	latest, found := LatestPrereleaseFromTags(tags, base, name)
	if !found {
		return base.WithPrerelease(name + ".1")
	}
	_, iter := ParsePrerelease(latest.Prerelease)
	return base.WithPrerelease(fmt.Sprintf("%s.%d", name, iter+1))
}

// BumpPastRetracted bumps patch, skipping any retracted versions.
func (v Version) BumpPastRetracted(retracted []Version) Version {
	next := v.BumpPatch()
	for IsRetracted(next, retracted) {
		next = next.BumpPatch()
	}
	return next
}
