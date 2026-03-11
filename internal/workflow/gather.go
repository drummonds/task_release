package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drummonds/task-plus/internal/cleanup"
	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/forge"
	"github.com/drummonds/task-plus/internal/git"
	"github.com/drummonds/task-plus/internal/version"
)

// Gather performs read-only state probing to populate the plan.
func Gather(ctx *Context) error {
	p := &ctx.Plan

	// Validate at least one git remote exists
	remotes, err := git.Remotes(ctx.Config.Dir)
	if err != nil {
		return fmt.Errorf("listing git remotes: %w", err)
	}
	if len(remotes) == 0 {
		return fmt.Errorf("no git remotes configured — add a remote before releasing")
	}

	// Validate configured push-target remotes exist
	for _, name := range ctx.Config.Remotes {
		if !git.HasRemote(ctx.Config.Dir, name) {
			return fmt.Errorf("configured remote %q not found in git", name)
		}
	}

	// If there's a docs repo, check it has a remote origin
	if docsDir := ctx.Config.ResolveDocsRepo(); docsDir != "" {
		docsRemotes, err := git.Remotes(docsDir)
		if err != nil {
			fmt.Printf("  WARN: cannot list -docs remotes: %v\n", err)
		} else if len(docsRemotes) == 0 {
			return fmt.Errorf("docs repo %s has no git remotes — add a remote before releasing", docsDir)
		}
	}

	// Validate language marker files
	if ctx.Config.HasGo() && !ctx.Config.HasGoMod() {
		return fmt.Errorf("language 'go' configured but no go.mod found")
	}
	if ctx.Config.HasPython() && !ctx.Config.HasPyproject() {
		return fmt.Errorf("language 'python' configured but no pyproject.toml found")
	}

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

	// Fork detection: compare go.mod module path vs primary remote
	if ctx.Config.Fork != nil {
		p.IsFork = *ctx.Config.Fork
	} else {
		modPath, err := version.ModulePath(ctx.Config.Dir)
		if err == nil {
			remotePath, err := version.GitRemoteModulePath(ctx.Config.Dir, ctx.Config.PrimaryRemote())
			if err == nil && remotePath != "" && remotePath != modPath {
				p.IsFork = true
			}
		}
	}
	if p.IsFork {
		branch, err := git.CurrentBranch(ctx.Config.Dir)
		if err != nil {
			return fmt.Errorf("getting current branch: %w", err)
		}
		p.ForkBranch = branch
		// Suggest pre-release version based on latest tag + branch name
		base := p.SuggestedVersion.Base()
		if found {
			base = latest
		}
		p.SuggestedVersion = base.BumpPrerelease(branch, tags)
	}

	// Taskfile release:version-update task?
	p.HasVersionUpdate = config.HasTaskfileTask(ctx.Config.Dir, "release:version-update")

	// Taskfile release:install task?
	p.HasReleaseInstall = config.HasTaskfileTask(ctx.Config.Dir, "release:install")

	// Goreleaser config exists?
	configPath := filepath.Join(ctx.Config.Dir, ctx.Config.GoreleaserConfig)
	if _, err := os.Stat(configPath); err == nil {
		p.HasGoreleaserCfg = true
	}

	// Detect forge from primary remote; check CLI availability for release cleanup.
	f, err := forge.Detect(ctx.Config.Dir, ctx.Config.PrimaryRemote(), ctx.Config.Forge)
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
