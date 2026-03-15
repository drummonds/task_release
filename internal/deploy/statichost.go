package deploy

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// Statichost deploys documentation to statichost.eu using their shcli tool.
type Statichost struct {
	Site string
}

func (s *Statichost) Name() string { return fmt.Sprintf("statichost.eu (%s)", s.Site) }

func (s *Statichost) Validate() error {
	return s.checkSiteExists()
}

func (s *Statichost) Deploy(projectDir, docsDir string, dryRun bool) error {
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		return fmt.Errorf("docs directory not found: %s", docsDir)
	}

	if dryRun {
		fmt.Printf("  (dry-run) Would deploy %s to statichost.eu site %q\n", docsDir, s.Site)
		return nil
	}

	shcli, err := ensureShcli()
	if err != nil {
		return err
	}

	if err := s.checkSiteExists(); err != nil {
		return err
	}

	fmt.Printf("  Deploying to statichost.eu site %q...\n", s.Site)
	cmd := exec.Command(shcli, s.Site, docsDir)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("shcli deploy failed: %w", err)
	}
	return nil
}

// checkSiteExists verifies the site exists on statichost.eu before deploying.
func (s *Statichost) checkSiteExists() error {
	apiKey := os.Getenv("STATICHOST_APIKEY")
	if apiKey == "" {
		return nil // shcli will report the missing key
	}
	builderHost := os.Getenv("STATICHOST_BUILDER")
	if builderHost == "" {
		builderHost = "https://builder.statichost.eu"
	}
	url := builderHost + "/" + s.Site + "/drop"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("checking site: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("checking site: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("site %q does not exist on statichost.eu — create it at %s first", s.Site, builderHost)
	}
	return nil
}

// ensureShcli returns the path to the shcli binary, downloading it if necessary.
func ensureShcli() (string, error) {
	// Check PATH first
	if path, err := exec.LookPath("shcli"); err == nil {
		return path, nil
	}

	// Check cache directory
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine cache directory: %w", err)
	}
	shcliDir := filepath.Join(cacheDir, "task-plus")
	shcliPath := filepath.Join(shcliDir, "shcli")

	if _, err := os.Stat(shcliPath); err == nil {
		return shcliPath, nil
	}

	// Download
	if _, err := exec.LookPath("curl"); err != nil {
		return "", fmt.Errorf("shcli not found and curl not available for download")
	}

	fmt.Println("  Downloading shcli from statichost.eu...")
	if err := os.MkdirAll(shcliDir, 0755); err != nil {
		return "", err
	}
	cmd := exec.Command("curl", "-fsSL", "-o", shcliPath, "https://www.statichost.eu/shcli")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("downloading shcli: %w", err)
	}
	if err := os.Chmod(shcliPath, 0755); err != nil {
		return "", err
	}

	return shcliPath, nil
}
