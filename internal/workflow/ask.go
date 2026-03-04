package workflow

import (
	"fmt"

	"github.com/drummonds/task-plus/internal/prompt"
	"github.com/drummonds/task-plus/internal/version"
)

// Ask presents all user prompts using gathered state, populating user decisions in the plan.
func Ask(ctx *Context) error {
	p := &ctx.Plan

	// Status summary
	if p.FoundTag {
		fmt.Printf("  Latest tag: %s\n", p.LatestTag)
	} else {
		fmt.Println("  No existing tags found.")
	}
	if len(p.Retracted) > 0 {
		fmt.Printf("  Retracted versions: %v\n", p.Retracted)
	}
	if p.IsFork {
		fmt.Printf("  Fork detected (branch: %s) — using pre-release versioning\n", p.ForkBranch)
	}
	fmt.Printf("  Suggested: %s\n", p.SuggestedVersion)

	if p.GitDirty {
		fmt.Println(p.StatusOutput)
		if !prompt.ConfirmOrAuto("Add all changes?") {
			return fmt.Errorf("aborted by user")
		}
		p.DoGitAdd = true
		p.CommitMsg = prompt.AskStringOrAuto("Commit message", "Release prep")
	} else {
		fmt.Println("  Working tree clean.")
	}

	// Version + comment
	input := prompt.AskStringOrAuto("Version", p.SuggestedVersion.String())
	v, err := version.Parse(input)
	if err != nil {
		return err
	}
	p.Version = v
	p.Comment = prompt.AskStringOrAuto("Release comment", p.CommitMsg)

	// Push
	p.DoPush = prompt.ConfirmOrAuto("Push to remote?")

	// Goreleaser
	if ctx.Config.IsBinary() && p.HasGoreleaserCfg {
		p.DoGoreleaser = prompt.ConfirmOrAuto("Run goreleaser?")
	}

	// Cleanup
	if len(p.ReleasesToDelete) > 0 {
		fmt.Printf("  Will delete %d old %s release(s):\n", len(p.ReleasesToDelete), p.Forge.Type)
		for _, d := range p.ReleasesToDelete {
			fmt.Printf("    - %s (%s)\n", d.Tag, d.Reason)
		}
		p.DoCleanup = prompt.ConfirmOrAuto("Delete these releases?")
	}

	// Install (binary projects or custom release:install task)
	if ctx.Config.IsBinary() || p.HasReleaseInstall {
		if ctx.Config.Install != nil {
			p.DoInstall = *ctx.Config.Install
		} else {
			p.DoInstall = prompt.ConfirmOrAuto("Install locally (go install)?")
		}
	}

	// Summary
	PrintSummary(ctx)

	return nil
}

// PrintSummary shows the planned actions before execution.
func PrintSummary(ctx *Context) {
	p := &ctx.Plan
	fmt.Println("\n--- Plan ---")
	if p.DoGitAdd {
		fmt.Printf("  Git add + commit: %q\n", p.CommitMsg)
	}
	fmt.Printf("  Version: %s\n", p.Version)
	if p.HasVersionUpdate {
		fmt.Println("  Version update: yes (Taskfile release:version-update)")
	}
	fmt.Printf("  Comment: %s\n", p.Comment)
	if p.DoPush {
		fmt.Println("  Push: yes")
	}
	if p.DoGoreleaser {
		fmt.Println("  Goreleaser: yes")
	}
	if p.DoCleanup {
		fmt.Printf("  Cleanup: delete %d releases\n", len(p.ReleasesToDelete))
	}
	if p.DoInstall && p.HasReleaseInstall {
		fmt.Println("  Install: yes (Taskfile release:install)")
	} else if p.DoInstall {
		fmt.Println("  Install: yes")
	}
	fmt.Println("---")
}
