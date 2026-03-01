package cleanup

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/drummonds/task-plus/internal/version"
)

// HasGH returns true if the gh CLI is available.
func HasGH() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// ListReleases returns release tag names from GitHub.
func ListReleases(dir string) ([]string, error) {
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

// PlanDeletions decides which releases to delete based on cleanup policy.
func PlanDeletions(tags []string, keepPatches, keepMinors int) []string {
	type parsed struct {
		tag string
		ver version.Version
	}

	var versions []parsed
	for _, t := range tags {
		v, err := version.Parse(t)
		if err == nil {
			versions = append(versions, parsed{t, v})
		}
	}

	// Sort newest first
	sort.Slice(versions, func(i, j int) bool {
		return versions[j].ver.Less(versions[i].ver)
	})

	// Group by minor version
	type minorKey struct{ major, minor int }
	groups := make(map[minorKey][]parsed)
	var minorOrder []minorKey
	for _, v := range versions {
		k := minorKey{v.ver.Major, v.ver.Minor}
		if _, ok := groups[k]; !ok {
			minorOrder = append(minorOrder, k)
		}
		groups[k] = append(groups[k], v)
	}

	var toDelete []string

	for i, k := range minorOrder {
		patches := groups[k]
		if i >= keepMinors {
			// Delete all releases in old minor versions
			for _, p := range patches {
				toDelete = append(toDelete, p.tag)
			}
			continue
		}
		// Keep only keepPatches per minor
		if len(patches) > keepPatches {
			for _, p := range patches[keepPatches:] {
				toDelete = append(toDelete, p.tag)
			}
		}
	}

	return toDelete
}

// DeleteRelease deletes a GitHub release by tag.
func DeleteRelease(dir, tag string) error {
	cmd := exec.Command("gh", "release", "delete", tag, "--yes", "--cleanup-tag")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh release delete %s: %w\n%s", tag, err, out)
	}
	return nil
}

// PrintPlan shows what would be deleted.
func PrintPlan(toDelete []string) {
	if len(toDelete) == 0 {
		fmt.Println("No old releases to clean up.")
		return
	}
	fmt.Printf("Will delete %d old release(s):\n", len(toDelete))
	for _, t := range toDelete {
		fmt.Printf("  - %s\n", t)
	}
}
