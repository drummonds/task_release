package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/deploy"
)

// validateDeploy pre-checks documentation deployment before any irreversible
// release steps. Verifies deploy targets are reachable and docs build succeeds.
func validateDeploy(ctx *Context) error {
	p := &ctx.Plan
	if !p.DoDeploy {
		return nil
	}

	deployDir := ctx.Config.Dir
	deployCfg := ctx.Config

	// Resolve -docs sibling (same logic as Execute)
	if !ctx.Config.IsDocs() {
		if docsRepoDir := ctx.Config.ResolveDocsRepo(); docsRepoDir != "" {
			docsCfg, err := config.Load(docsRepoDir)
			if err != nil {
				return fmt.Errorf("loading docs config: %w", err)
			}
			deployDir = docsRepoDir
			deployCfg = docsCfg
		}
	}

	// Validate each deploy target is reachable
	for _, target := range deployCfg.PagesDeploy {
		d, err := deploy.New(target)
		if err != nil {
			return fmt.Errorf("deploy target: %w", err)
		}
		fmt.Printf("  Checking %s...\n", d.Name())
		if err := d.Validate(); err != nil {
			return fmt.Errorf("deploy target %s: %w", d.Name(), err)
		}
	}

	// Dry-run the docs build to catch broken commands early
	if deployCfg.HasPagesBuild() {
		fmt.Println("  Test-building documentation...")
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
				return fmt.Errorf("docs build pre-check failed: %w", err)
			}
		}
	}

	return nil
}
