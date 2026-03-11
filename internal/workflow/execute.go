package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/drummonds/task-plus/internal/changelog"
	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/deploy"
	"github.com/drummonds/task-plus/internal/git"
	"github.com/drummonds/task-plus/internal/readme"
	"github.com/drummonds/task-plus/internal/release"
	"github.com/drummonds/task-plus/internal/version"
)

// rollback tracks state for undoing local mutations on failure.
type rollback struct {
	dir        string
	origHEAD   string
	tagCreated string
	pushed     bool
}

// undo resets the repo to its pre-release state if we haven't pushed yet.
func (rb *rollback) undo() {
	if rb.pushed {
		fmt.Println("  Rollback: changes already pushed — manual cleanup required")
		return
	}
	if rb.tagCreated != "" {
		fmt.Printf("  Rollback: deleting tag %s\n", rb.tagCreated)
		git.Run(rb.dir, "tag", "-d", rb.tagCreated)
	}
	if rb.origHEAD != "" {
		fmt.Printf("  Rollback: resetting to %s\n", rb.origHEAD)
		git.Run(rb.dir, "reset", "--hard", rb.origHEAD)
	}
}

// Execute performs all mutations based on the plan. No prompts.
// On failure before push, rolls back local changes (reset + delete tag).
func Execute(ctx *Context) error {
	rb := &rollback{dir: ctx.Config.Dir}

	// Record HEAD before mutations (skip in dry-run)
	if !ctx.DryRun {
		head, err := git.Run(ctx.Config.Dir, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("reading HEAD: %w", err)
		}
		rb.origHEAD = head
	}

	if err := executeSteps(ctx, rb); err != nil {
		if !ctx.DryRun {
			rb.undo()
		}
		return err
	}
	return nil
}

// executeSteps contains the actual release steps.
func executeSteps(ctx *Context, rb *rollback) error {
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

	// 4. Run release:version-update Taskfile task if present
	if p.HasVersionUpdate {
		fmt.Printf("  Running release:version-update with VERSION=%s\n", p.Version)
		if ctx.DryRun {
			fmt.Printf("  (dry-run) Would run: task release:version-update\n")
		} else {
			cmd := exec.Command("task", "release:version-update")
			cmd.Dir = ctx.Config.Dir
			cmd.Env = append(os.Environ(), "VERSION="+p.Version.String())
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("release:version-update failed: %w", err)
			}
			// Commit version-update changes if any
			clean, _ := git.IsClean(ctx.Config.Dir)
			if !clean {
				if err := git.AddAll(ctx.Config.Dir); err != nil {
					return err
				}
				if err := git.Commit(ctx.Config.Dir, fmt.Sprintf("Update version to %s", p.Version)); err != nil {
					return err
				}
			}
		}
	}

	// 4b. Update README.md auto-marker sections (if markers present)
	fmt.Println("  Updating README.md markers...")
	if ctx.DryRun {
		fmt.Println("  (dry-run) Would update README.md markers")
	} else {
		if err := readme.Update(ctx.Config.Dir, p.Version.String()); err != nil {
			fmt.Printf("  Warning: README.md update: %v\n", err)
			// Non-fatal: continue the release
		} else {
			clean, _ := git.IsClean(ctx.Config.Dir)
			if !clean {
				if err := git.AddAll(ctx.Config.Dir); err != nil {
					return err
				}
				if err := git.Commit(ctx.Config.Dir, fmt.Sprintf("Update README.md for %s", p.Version)); err != nil {
					return err
				}
			}
		}
	}

	// 5. Update changelog + auto-commit
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

	// 6. Git tag
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
		rb.tagCreated = tag
	}

	// 7. WASM build
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

	// 8. Git push (all configured remotes) — ROLLBACK BOUNDARY
	if p.DoPush {
		for _, remote := range ctx.Config.Remotes {
			fmt.Printf("  Pushing to %s...\n", remote)
			if ctx.DryRun {
				fmt.Printf("  (dry-run) Would push branch and tags to %s\n", remote)
			} else {
				if err := git.PushTo(ctx.Config.Dir, remote); err != nil {
					return fmt.Errorf("push to %s: %w", remote, err)
				}
				rb.pushed = true
			}
		}
	}

	// 9. Goreleaser (post-push — warn only on failure)
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

	// 9b. PyPI publish (post-push)
	if p.DoPublishPyPI {
		fmt.Println("  Publishing to PyPI...")
		if ctx.DryRun {
			fmt.Println("  (dry-run) Would run: uv build && uv publish")
		} else {
			// Build
			build := exec.Command("uv", "build")
			build.Dir = ctx.Config.Dir
			build.Stdout = os.Stdout
			build.Stderr = os.Stderr
			if err := build.Run(); err != nil {
				fmt.Printf("  Warning: uv build failed: %v\n", err)
			} else {
				// Publish
				publish := exec.Command("uv", "publish")
				publish.Dir = ctx.Config.Dir
				publish.Stdout = os.Stdout
				publish.Stderr = os.Stderr
				if err := publish.Run(); err != nil {
					fmt.Printf("  Warning: uv publish failed: %v\n", err)
				}
			}
		}
	}

	// 10. Cleanup
	if p.DoCleanup {
		fmt.Println("  Cleaning up old releases...")
		if ctx.DryRun {
			fmt.Println("  (dry-run) Would delete releases")
		} else {
			for _, d := range p.ReleasesToDelete {
				fmt.Printf("  Deleting %s (%s)...\n", d.Tag, d.Reason)
				if err := p.Forge.DeleteRelease(ctx.Config.Dir, d.Tag); err != nil {
					fmt.Printf("  Warning: %v\n", err)
				}
			}
		}
	}

	// 11. Local install (bypass proxy to avoid stale cache after tag push)
	if p.DoInstall && p.HasReleaseInstall {
		// Custom install via Taskfile release:install
		fmt.Printf("  Running release:install with VERSION=%s\n", p.Version)
		if ctx.DryRun {
			fmt.Println("  (dry-run) Would run: task release:install")
		} else {
			cmd := exec.Command("task", "release:install")
			cmd.Dir = ctx.Config.Dir
			cmd.Env = append(os.Environ(), "VERSION="+p.Version.String())
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("release:install failed: %w", err)
			}
		}
	} else if p.DoInstall {
		modPath, err := version.ModulePath(ctx.Config.Dir)
		if err != nil {
			return fmt.Errorf("reading module path: %w", err)
		}
		if p.IsFork {
			fmt.Printf("  Skipping install: fork detected (branch: %s). Install manually if needed.\n", p.ForkBranch)
		} else {
			// Use cmd/... if cmd/ directory exists, otherwise install the root package
			installArg := modPath + "@" + p.Version.String()
			if info, serr := os.Stat(filepath.Join(ctx.Config.Dir, "cmd")); serr == nil && info.IsDir() {
				installArg = modPath + "/cmd/...@" + p.Version.String()
			}
			fmt.Printf("  Installing %s ...\n", installArg)
			if ctx.DryRun {
				fmt.Printf("  (dry-run) Would run GOPROXY=direct go install %s\n", installArg)
			} else {
				retries := ctx.Config.InstallRetries
				var lastErr error
				for attempt := 1; attempt <= retries; attempt++ {
					cmd := exec.Command("go", "install", installArg)
					cmd.Env = append(os.Environ(), "GOPROXY=direct")
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					lastErr = cmd.Run()
					if lastErr == nil {
						break
					}
					if attempt < retries {
						delay := time.Duration(attempt*5) * time.Second
						fmt.Printf("  Install attempt %d/%d failed, retrying in %s...\n", attempt, retries, delay)
						time.Sleep(delay)
					}
				}
				if lastErr != nil {
					return fmt.Errorf("go install failed after %d attempts: %w", retries, lastErr)
				}
			}
		}
	}

	// 12. Pages deploy (delegates to -docs sibling if available)
	if p.DoDeploy {
		deployDir := ctx.Config.Dir
		deployCfg := ctx.Config

		// Check for -docs sibling
		if !ctx.Config.IsDocs() {
			if docsRepoDir := ctx.Config.ResolveDocsRepo(); docsRepoDir != "" {
				fmt.Printf("  Using docs repo: %s\n", docsRepoDir)
				docsCfg, err := config.Load(docsRepoDir)
				if err != nil {
					return fmt.Errorf("loading docs config: %w", err)
				}
				deployDir = docsRepoDir
				deployCfg = docsCfg
			}
		}

		// Build docs first if configured
		if deployCfg.HasPagesBuild() {
			fmt.Println("  Building documentation...")
			for _, cmd := range deployCfg.PagesBuild {
				fmt.Printf("  $ %s\n", cmd)
				if ctx.DryRun {
					continue
				}
				parts := strings.Fields(cmd)
				c := exec.Command(parts[0], parts[1:]...)
				c.Dir = deployDir
				c.Env = append(os.Environ(), "TP_VERSION="+p.Version.String())
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				if err := c.Run(); err != nil {
					return fmt.Errorf("docs build failed: %w", err)
				}
			}
		}

		docsDir := filepath.Join(deployDir, "docs")
		for _, target := range deployCfg.PagesDeploy {
			d, err := deploy.New(target)
			if err != nil {
				return err
			}
			fmt.Printf("  Deploying to %s...\n", d.Name())
			if err := d.Deploy(deployDir, docsDir, ctx.DryRun); err != nil {
				return err
			}
		}
	}

	return nil
}
