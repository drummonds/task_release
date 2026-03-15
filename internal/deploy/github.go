package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// GitHub deploys documentation to GitHub Pages by pushing docs/ to the gh-pages branch.
type GitHub struct{}

func (g *GitHub) Name() string { return "GitHub Pages" }

func (g *GitHub) Validate() error { return nil }

func (g *GitHub) Deploy(projectDir, docsDir string, dryRun bool) error {
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		return fmt.Errorf("docs directory not found: %s", docsDir)
	}

	rel, err := filepath.Rel(projectDir, docsDir)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("  (dry-run) Would push %s/ to gh-pages branch\n", rel)
		return nil
	}

	fmt.Printf("  Pushing %s/ to gh-pages branch...\n", rel)
	cmd := exec.Command("git", "subtree", "push", "--prefix", rel, "origin", "gh-pages")
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git subtree push failed: %w", err)
	}
	return nil
}
