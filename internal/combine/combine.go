// Package combine merges a -docs sibling repo back into the main project repo.
// This reverses the split-repo pattern, consolidating docs into the main repo.
package combine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/drummonds/task-plus/internal/config"
	"github.com/drummonds/task-plus/internal/deploy"
	"gopkg.in/yaml.v3"
)

// Run merges the -docs sibling back into the main project at dir.
// Copies docs content, merges pages_build/pages_deploy config, and reports next steps.
func Run(dir string) error {
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}

	if cfg.IsDocs() {
		return fmt.Errorf("this is a docs project — run combine from the main project")
	}

	docsDir := cfg.ResolveDocsRepo()
	if docsDir == "" {
		return fmt.Errorf("no -docs sibling found")
	}

	docsCfg, err := config.Load(docsDir)
	if err != nil {
		return fmt.Errorf("loading -docs config: %w", err)
	}

	fmt.Printf("Combining %s into %s\n", docsDir, dir)

	// Copy docs/ from -docs repo to main repo
	srcDocs := filepath.Join(docsDir, "docs")
	dstDocs := filepath.Join(dir, "docs")
	if _, err := os.Stat(srcDocs); err == nil {
		fmt.Println("  Copying docs/ contents...")
		if err := os.MkdirAll(dstDocs, 0755); err != nil {
			return err
		}
		if err := copyDir(srcDocs, dstDocs); err != nil {
			return fmt.Errorf("copying docs: %w", err)
		}
	}

	// Copy any DOC- prefixed files, renaming to remove prefix
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		return fmt.Errorf("reading -docs dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "DOC-") {
			continue
		}
		newName := strings.TrimPrefix(name, "DOC-")
		src := filepath.Join(docsDir, name)
		dst := filepath.Join(dir, newName)
		// Don't overwrite existing files in main repo
		if _, err := os.Stat(dst); err == nil {
			fmt.Printf("  Skipping %s (already exists in main repo)\n", newName)
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", newName, err)
		}
		fmt.Printf("  Copied %s → %s\n", name, newName)
	}

	// Copy any extra markdown/source files from -docs root that aren't standard scaffolding
	docsRepoScaffold := map[string]bool{
		"task-plus.yml": true,
		"Taskfile.yml":  true,
		"README.md":     true,
		".gitignore":    true,
		".git":          true,
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if docsRepoScaffold[name] || strings.HasPrefix(name, "DOC-") || strings.HasPrefix(name, ".") {
			continue
		}
		// Copy non-scaffold, non-DOC files into docs/ subdirectory
		if strings.HasSuffix(strings.ToLower(name), ".md") {
			src := filepath.Join(docsDir, name)
			dst := filepath.Join(dstDocs, name)
			if _, err := os.Stat(dst); err == nil {
				continue // already exists
			}
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			_ = os.MkdirAll(dstDocs, 0755)
			if err := os.WriteFile(dst, data, 0644); err != nil {
				continue
			}
			fmt.Printf("  Copied %s → docs/%s\n", name, name)
		}
	}

	// Merge pages_build and pages_deploy into main config
	if err := mergeConfig(dir, docsCfg); err != nil {
		return fmt.Errorf("merging config: %w", err)
	}

	fmt.Printf("\nCombined docs from %s\n", filepath.Base(docsDir))
	fmt.Println("Next steps:")
	fmt.Println("  1. Review the merged docs/ and task-plus.yml")
	fmt.Println("  2. Run 'tp pages' to verify local serving")
	fmt.Println("  3. Run 'tp pages deploy' to verify deployment")
	fmt.Printf("  4. Archive/delete %s when satisfied\n", filepath.Base(docsDir))
	return nil
}

// mergeConfig adds pages_build and pages_deploy from docs config into main config,
// and removes docs_repo/parent_repo fields.
func mergeConfig(dir string, docsCfg *config.Config) error {
	path := filepath.Join(dir, "task-plus.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	// Parse existing config
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	// Add pages_deploy from docs config if main doesn't have it
	if len(docsCfg.PagesDeploy) > 0 {
		if _, has := raw["pages_deploy"]; !has {
			raw["pages_deploy"] = marshalTargets(docsCfg.PagesDeploy)
			fmt.Println("  Added pages_deploy from -docs config")
		}
	}

	// Add pages_build from docs config if main doesn't have it
	if len(docsCfg.PagesBuild) > 0 {
		if _, has := raw["pages_build"]; !has {
			raw["pages_build"] = docsCfg.PagesBuild
			fmt.Println("  Added pages_build from -docs config")
		}
	}

	// Remove docs_repo and parent_repo
	delete(raw, "docs_repo")
	delete(raw, "parent_repo")

	out, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// marshalTargets converts deploy targets to a yaml-friendly format.
func marshalTargets(targets []deploy.Target) []map[string]string {
	var result []map[string]string
	for _, t := range targets {
		m := map[string]string{"type": t.Type}
		if t.Site != "" {
			m["site"] = t.Site
		}
		result = append(result, m)
	}
	return result
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		// Don't overwrite existing files
		if _, err := os.Stat(target); err == nil {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
