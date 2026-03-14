package forge

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// forgejoRelease is the subset of the Forgejo/Gitea release API response we need.
type forgejoRelease struct {
	ID      int    `json:"id"`
	TagName string `json:"tag_name"`
}

func listReleasesForgejo(remoteURL string) ([]string, error) {
	host, owner, repo := ExtractOwnerRepo(remoteURL)
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("cannot parse owner/repo from %q", remoteURL)
	}
	apiURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases?limit=100", host, owner, repo)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	if token := os.Getenv("CODEBERG_APIKEY"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forgejo API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("forgejo API %s: %s", resp.Status, body)
	}

	var releases []forgejoRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("forgejo API decode: %w", err)
	}

	var tags []string
	for _, r := range releases {
		tags = append(tags, r.TagName)
	}
	return tags, nil
}

func deleteReleaseForgejo(remoteURL, tag string) error {
	host, owner, repo := ExtractOwnerRepo(remoteURL)
	if owner == "" || repo == "" {
		return fmt.Errorf("cannot parse owner/repo from %q", remoteURL)
	}
	token := os.Getenv("CODEBERG_APIKEY")
	if token == "" {
		return fmt.Errorf("CODEBERG_APIKEY not set")
	}

	// Find release ID by tag
	apiURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/tags/%s", host, owner, repo, tag)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("forgejo API: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("forgejo API get release %s: %s %s", tag, resp.Status, body)
	}

	var rel forgejoRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("forgejo API decode: %w", err)
	}

	// Delete the release
	delURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/releases/%d", host, owner, repo, rel.ID)
	delReq, err := http.NewRequest("DELETE", delURL, nil)
	if err != nil {
		return err
	}
	delReq.Header.Set("Authorization", "token "+token)

	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		return fmt.Errorf("forgejo API delete release: %w", err)
	}
	_ = delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent && delResp.StatusCode != http.StatusOK {
		return fmt.Errorf("forgejo API delete release %s: %s", tag, delResp.Status)
	}

	// Delete the tag
	tagURL := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/tags/%s", host, owner, repo, tag)
	tagReq, err := http.NewRequest("DELETE", tagURL, nil)
	if err != nil {
		return err
	}
	tagReq.Header.Set("Authorization", "token "+token)

	tagResp, err := http.DefaultClient.Do(tagReq)
	if err != nil {
		return fmt.Errorf("forgejo API delete tag: %w", err)
	}
	_ = tagResp.Body.Close()
	// Tag deletion may return 204 or 404 (already gone via release delete)

	return nil
}
