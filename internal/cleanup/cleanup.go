package cleanup

import (
	"fmt"
	"sort"

	"github.com/drummonds/task-plus/internal/version"
)

// Deletion describes a release to delete and the reason why.
type Deletion struct {
	Tag    string
	Reason string
}

// PlanDeletions decides which releases to delete based on cleanup policy.
func PlanDeletions(tags []string, keepPatches, keepMinors int) []Deletion {
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

	// Build the "keeping" label for old-minor reason
	var keptLabel string
	if keepMinors > 0 && len(minorOrder) > 0 {
		newest := minorOrder[0]
		if keepMinors == 1 {
			keptLabel = fmt.Sprintf("keeping %d.%d.x", newest.major, newest.minor)
		} else {
			last := keepMinors - 1
			if last >= len(minorOrder) {
				last = len(minorOrder) - 1
			}
			oldest := minorOrder[last]
			keptLabel = fmt.Sprintf("keeping %d.%d.x–%d.%d.x",
				newest.major, newest.minor, oldest.major, oldest.minor)
		}
	}

	var toDelete []Deletion

	for i, k := range minorOrder {
		patches := groups[k]
		if i >= keepMinors {
			// Delete all releases in old minor versions
			reason := fmt.Sprintf("old minor version (%s)", keptLabel)
			for _, p := range patches {
				toDelete = append(toDelete, Deletion{p.tag, reason})
			}
			continue
		}
		// Keep only keepPatches per minor
		if len(patches) > keepPatches {
			for _, p := range patches[keepPatches:] {
				toDelete = append(toDelete, Deletion{p.tag, "old patch version"})
			}
		}
	}

	return toDelete
}

// PrintPlan shows what would be deleted.
func PrintPlan(toDelete []Deletion) {
	if len(toDelete) == 0 {
		fmt.Println("No old releases to clean up.")
		return
	}
	fmt.Printf("Will delete %d old release(s):\n", len(toDelete))
	for _, d := range toDelete {
		fmt.Printf("  - %s (%s)\n", d.Tag, d.Reason)
	}
}
