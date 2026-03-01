package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/drummonds/task-plus/internal/changelog"
	"github.com/drummonds/task-plus/internal/cleanup"
	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/git"
	"github.com/drummonds/task-plus/internal/prompt"
	"github.com/drummonds/task-plus/internal/release"
	"github.com/drummonds/task-plus/internal/version"
)

type Context struct {
	Config    *config.Config
	DryRun    bool
	Version   version.Version
	Comment   string
	CommitMsg string
}

// Run executes the full release workflow.
func Run(cfg *config.Config, dryRun bool) error {
	ctx := &Context{Config: cfg, DryRun: dryRun}

	steps := []struct {
		name string
		fn   func(*Context) error
	}{
		{"Run checks", stepRunChecks},
		{"Show status", stepShowStatus},
		{"Git add", stepGitAdd},
		{"Git commit", stepGitCommit},
		{"Detect version", stepDetectVersion},
		{"Update changelog", stepUpdateChangelog},
		{"Git tag", stepGitTag},
		{"WASM build", stepWasmBuild},
		{"Git push", stepGitPush},
		{"Goreleaser", stepGoreleaser},
		{"Cleanup", stepCleanup},
		{"Local install", stepLocalInstall},
	}

	for _, s := range steps {
		fmt.Printf("\n=== %s ===\n", s.name)
		if err := s.fn(ctx); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}

	fmt.Printf("\nRelease %s complete!\n", ctx.Version)
	return nil
}

func stepRunChecks(ctx *Context) error {
	for _, cmd := range ctx.Config.Check {
		fmt.Printf("  $ %s\n", cmd)
		if ctx.DryRun {
			continue
		}
		parts := strings.Fields(cmd)
		c := exec.Command(parts[0], parts[1:]...)
		c.Dir = ctx.Config.Dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", cmd, err)
		}
	}
	return nil
}

func stepShowStatus(ctx *Context) error {
	out, err := git.Status(ctx.Config.Dir)
	if err != nil {
		return err
	}
	if out == "" {
		fmt.Println("  Working tree clean.")
	} else {
		fmt.Println(out)
	}
	return nil
}

func stepGitAdd(ctx *Context) error {
	clean, err := git.IsClean(ctx.Config.Dir)
	if err != nil {
		return err
	}
	if clean {
		fmt.Println("  Nothing to add.")
		return nil
	}
	if !prompt.ConfirmOrAuto("Add all changes?") {
		return fmt.Errorf("aborted by user")
	}
	if ctx.DryRun {
		fmt.Println("  (dry-run) Would git add -A")
		return nil
	}
	return git.AddAll(ctx.Config.Dir)
}

func stepGitCommit(ctx *Context) error {
	clean, err := git.IsClean(ctx.Config.Dir)
	if err != nil {
		return err
	}
	if clean {
		fmt.Println("  Nothing to commit.")
		return nil
	}
	msg := prompt.AskStringOrAuto("Commit message", "Release prep")
	ctx.CommitMsg = msg
	if ctx.DryRun {
		fmt.Printf("  (dry-run) Would commit: %q\n", msg)
		return nil
	}
	return git.Commit(ctx.Config.Dir, msg)
}

func stepDetectVersion(ctx *Context) error {
	tags, err := git.Tags(ctx.Config.Dir)
	if err != nil {
		return err
	}

	latest, found := version.LatestFromTags(tags)
	var suggested version.Version
	if found {
		suggested = latest.BumpPatch()
		fmt.Printf("  Latest tag: %s\n", latest)
	} else {
		suggested = version.Version{Major: 0, Minor: 1, Patch: 0}
		fmt.Println("  No existing tags found.")
	}
	fmt.Printf("  Suggested: %s\n", suggested)

	input := prompt.AskStringOrAuto("Version", suggested.String())
	v, err := version.Parse(input)
	if err != nil {
		return err
	}

	// Check tag doesn't already exist
	exists, err := git.TagExists(ctx.Config.Dir, v.String())
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("tag %s already exists", v)
	}

	ctx.Version = v

	// Ask for release comment (default to commit message if available)
	ctx.Comment = prompt.AskStringOrAuto("Release comment", ctx.CommitMsg)
	return nil
}

func stepUpdateChangelog(ctx *Context) error {
	fmt.Printf("  Adding %s to CHANGELOG.md (%s format)\n", ctx.Version.TagString(), ctx.Config.ChangelogFormat)
	if ctx.DryRun {
		fmt.Println("  (dry-run) Would update changelog")
		return nil
	}
	if err := changelog.Update(ctx.Config.Dir, ctx.Version.TagString(), ctx.Config.ChangelogFormat, ctx.Comment); err != nil {
		return err
	}
	// Auto-commit the changelog update
	if err := git.AddAll(ctx.Config.Dir); err != nil {
		return err
	}
	return git.Commit(ctx.Config.Dir, fmt.Sprintf("Update CHANGELOG for %s", ctx.Version))
}

func stepGitTag(ctx *Context) error {
	tag := ctx.Version.String()
	msg := tag
	if ctx.Comment != "" {
		msg = ctx.Comment
	}
	fmt.Printf("  Tagging %s\n", tag)
	if ctx.DryRun {
		fmt.Printf("  (dry-run) Would tag %s\n", tag)
		return nil
	}
	return git.Tag(ctx.Config.Dir, tag, msg)
}

func stepWasmBuild(ctx *Context) error {
	if !ctx.Config.HasWasm() {
		fmt.Println("  No WASM build configured, skipping.")
		return nil
	}
	for _, cmd := range ctx.Config.Wasm {
		fmt.Printf("  $ %s\n", cmd)
		if ctx.DryRun {
			continue
		}
		parts := strings.Fields(cmd)
		c := exec.Command(parts[0], parts[1:]...)
		c.Dir = ctx.Config.Dir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", cmd, err)
		}
	}
	// If WASM artifacts were built, commit them
	if !ctx.DryRun {
		clean, _ := git.IsClean(ctx.Config.Dir)
		if !clean {
			if err := git.AddAll(ctx.Config.Dir); err != nil {
				return err
			}
			if err := git.Commit(ctx.Config.Dir, fmt.Sprintf("Build WASM for %s", ctx.Version)); err != nil {
				return err
			}
			// Move tag to include WASM commit
			// Delete old tag and recreate
			git.Run(ctx.Config.Dir, "tag", "-d", ctx.Version.String())
			msg := ctx.Version.String()
			if ctx.Comment != "" {
				msg = ctx.Comment
			}
			git.Tag(ctx.Config.Dir, ctx.Version.String(), msg)
		}
	}
	return nil
}

func stepGitPush(ctx *Context) error {
	if !prompt.ConfirmOrAuto("Push to remote?") {
		return fmt.Errorf("aborted by user")
	}
	if ctx.DryRun {
		fmt.Println("  (dry-run) Would push branch and tags")
		return nil
	}
	return git.Push(ctx.Config.Dir)
}

func stepGoreleaser(ctx *Context) error {
	if !ctx.Config.IsBinary() {
		fmt.Println("  Library project, skipping goreleaser.")
		return nil
	}
	if !prompt.ConfirmOrAuto("Run goreleaser?") {
		fmt.Println("  Skipped.")
		return nil
	}
	if ctx.DryRun {
		fmt.Println("  (dry-run) Would run goreleaser")
		return nil
	}
	return release.RunGoreleaser(ctx.Config.Dir, ctx.Config.GoreleaserConfig)
}

func stepCleanup(ctx *Context) error {
	if !cleanup.HasGH() {
		fmt.Println("  gh CLI not found, skipping cleanup.")
		return nil
	}

	tags, err := cleanup.ListReleases(ctx.Config.Dir)
	if err != nil {
		fmt.Printf("  Warning: %v\n", err)
		return nil
	}

	toDelete := cleanup.PlanDeletions(tags, ctx.Config.Cleanup.KeepPatches, ctx.Config.Cleanup.KeepMinors)
	cleanup.PrintPlan(toDelete)

	if len(toDelete) == 0 {
		return nil
	}

	if !prompt.ConfirmOrAuto("Delete these releases?") {
		fmt.Println("  Skipped.")
		return nil
	}

	if ctx.DryRun {
		fmt.Println("  (dry-run) Would delete releases")
		return nil
	}

	for _, tag := range toDelete {
		fmt.Printf("  Deleting %s...\n", tag)
		if err := cleanup.DeleteRelease(ctx.Config.Dir, tag); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		}
	}
	return nil
}

func stepLocalInstall(ctx *Context) error {
	if ctx.Config.Install != nil {
		if !*ctx.Config.Install {
			fmt.Println("  Skipped (config: install=false).")
			return nil
		}
	} else if !prompt.ConfirmOrAuto("Install locally (go install)?") {
		fmt.Println("  Skipped.")
		return nil
	}
	if ctx.DryRun {
		fmt.Println("  (dry-run) Would run go install")
		return nil
	}
	cmd := exec.Command("go", "install", "./cmd/...")
	cmd.Dir = ctx.Config.Dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
