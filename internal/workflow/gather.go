package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drummonds/task-plus/internal/cleanup"
	"github.com/drummonds/task-plus/internal/forge"
	"github.com/drummonds/task-plus/internal/git"
	"github.com/drummonds/task-plus/internal/version"
)

// Gather performs read-only state probing to populate the plan.
func Gather(ctx *Context) error {
	p := &ctx.Plan

	// Git status
	out, err := git.Status(ctx.Config.Dir)
	if err != nil {
		return err
	}
	p.StatusOutput = out
	p.GitDirty = out != ""

	// Tags + retracted versions → suggested version
	tags, err := git.Tags(ctx.Config.Dir)
	if err != nil {
		return err
	}

	retracted, err := version.ParseRetracted(ctx.Config.Dir)
	if err != nil {
		return fmt.Errorf("parsing retracted versions: %w", err)
	}
	p.Retracted = retracted

	latest, found := version.LatestFromTags(tags, retracted)
	p.LatestTag = latest
	p.FoundTag = found
	if found {
		p.SuggestedVersion = latest.BumpPastRetracted(retracted)
	} else {
		p.SuggestedVersion = version.Version{Major: 0, Minor: 1, Patch: 0}
	}

	// Goreleaser config exists?
	configPath := filepath.Join(ctx.Config.Dir, ctx.Config.GoreleaserConfig)
	if _, err := os.Stat(configPath); err == nil {
		p.HasGoreleaserCfg = true
	}

	// Detect forge and check CLI availability for release cleanup.
	f, err := forge.Detect(ctx.Config.Dir, ctx.Config.Forge)
	if err != nil {
		return fmt.Errorf("detecting forge: %w", err)
	}
	p.Forge = f
	p.HasForgeCLI = f.HasCLI()
	if p.HasForgeCLI {
		releases, err := f.ListReleases(ctx.Config.Dir)
		if err == nil {
			p.ReleasesToDelete = cleanup.PlanDeletions(releases, ctx.Config.Cleanup.KeepPatches, ctx.Config.Cleanup.KeepMinors)
		}
	}

	return nil
}
