package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/drummonds/task-plus/internal/changelog"
	"github.com/drummonds/task-plus/internal/cleanup"
	"github.com/drummonds/task-plus/internal/git"
	"github.com/drummonds/task-plus/internal/release"
)

// Execute performs all mutations based on the plan. No prompts.
func Execute(ctx *Context) error {
	p := &ctx.Plan

	// 1. Git add
	if p.DoGitAdd {
		fmt.Println("  Git add...")
		if ctx.DryRun {
			fmt.Println("  (dry-run) Would git add -A")
		} else {
			if err := git.AddAll(ctx.Config.Dir); err != nil {
				return err
			}
		}
	}

	// 2. Git commit
	if p.DoGitAdd && p.GitDirty {
		fmt.Printf("  Git commit: %q\n", p.CommitMsg)
		if ctx.DryRun {
			fmt.Printf("  (dry-run) Would commit: %q\n", p.CommitMsg)
		} else {
			if err := git.Commit(ctx.Config.Dir, p.CommitMsg); err != nil {
				return err
			}
		}
	}

	// 3. Check tag doesn't already exist
	exists, err := git.TagExists(ctx.Config.Dir, p.Version.String())
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("tag %s already exists", p.Version)
	}

	// 4. Update changelog + auto-commit
	fmt.Printf("  Updating CHANGELOG.md (%s format)\n", ctx.Config.ChangelogFormat)
	if ctx.DryRun {
		fmt.Println("  (dry-run) Would update changelog")
	} else {
		if err := changelog.Update(ctx.Config.Dir, p.Version.TagString(), ctx.Config.ChangelogFormat, p.Comment); err != nil {
			return err
		}
		if err := git.AddAll(ctx.Config.Dir); err != nil {
			return err
		}
		if err := git.Commit(ctx.Config.Dir, fmt.Sprintf("Update CHANGELOG for %s", p.Version)); err != nil {
			return err
		}
	}

	// 5. Git tag
	tag := p.Version.String()
	msg := tag
	if p.Comment != "" {
		msg = p.Comment
	}
	fmt.Printf("  Tagging %s\n", tag)
	if ctx.DryRun {
		fmt.Printf("  (dry-run) Would tag %s\n", tag)
	} else {
		if err := git.Tag(ctx.Config.Dir, tag, msg); err != nil {
			return err
		}
	}

	// 6. WASM build
	if ctx.Config.HasWasm() {
		fmt.Println("  Building WASM...")
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
		// If WASM artifacts were built, commit and re-tag
		if !ctx.DryRun {
			clean, _ := git.IsClean(ctx.Config.Dir)
			if !clean {
				if err := git.AddAll(ctx.Config.Dir); err != nil {
					return err
				}
				if err := git.Commit(ctx.Config.Dir, fmt.Sprintf("Build WASM for %s", p.Version)); err != nil {
					return err
				}
				// Move tag to include WASM commit
				git.Run(ctx.Config.Dir, "tag", "-d", p.Version.String())
				tagMsg := p.Version.String()
				if p.Comment != "" {
					tagMsg = p.Comment
				}
				git.Tag(ctx.Config.Dir, p.Version.String(), tagMsg)
			}
		}
	}

	// 7. Git push
	if p.DoPush {
		fmt.Println("  Pushing...")
		if ctx.DryRun {
			fmt.Println("  (dry-run) Would push branch and tags")
		} else {
			if err := git.Push(ctx.Config.Dir); err != nil {
				return err
			}
		}
	}

	// 8. Goreleaser
	if p.DoGoreleaser {
		fmt.Println("  Running goreleaser...")
		if ctx.DryRun {
			fmt.Println("  (dry-run) Would run goreleaser")
		} else {
			if err := release.RunGoreleaser(ctx.Config.Dir, ctx.Config.GoreleaserConfig); err != nil {
				return err
			}
		}
	}

	// 9. Cleanup
	if p.DoCleanup {
		fmt.Println("  Cleaning up old releases...")
		if ctx.DryRun {
			fmt.Println("  (dry-run) Would delete releases")
		} else {
			for _, tag := range p.ReleasesToDelete {
				fmt.Printf("  Deleting %s...\n", tag)
				if err := cleanup.DeleteRelease(ctx.Config.Dir, tag); err != nil {
					fmt.Printf("  Warning: %v\n", err)
				}
			}
		}
	}

	// 10. Local install
	if p.DoInstall {
		fmt.Println("  Installing locally...")
		if ctx.DryRun {
			fmt.Println("  (dry-run) Would run go install")
		} else {
			cmd := exec.Command("go", "install", "./cmd/...")
			cmd.Dir = ctx.Config.Dir
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return err
			}
		}
	}

	return nil
}
