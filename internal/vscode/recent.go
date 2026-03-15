// Package vscode provides helpers for managing VS Code state.
package vscode

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// stateDBPath returns the path to VS Code's state.vscdb.
func stateDBPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "Code", "User", "globalStorage", "state.vscdb")
}

// RemoveFromRecent removes a folder path from VS Code's recently opened list.
// All errors are silently ignored — this is best-effort cleanup.
func RemoveFromRecent(folderPath string) error {
	dbPath := stateDBPath()
	if dbPath == "" {
		return fmt.Errorf("cannot determine VS Code state path")
	}
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("state.vscdb not found: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open state.vscdb: %w", err)
	}
	defer db.Close()

	const key = "history.recentlyOpenedPathsList"

	var raw string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", key).Scan(&raw)
	if err != nil {
		return fmt.Errorf("read recent paths: %w", err)
	}

	// Parse as generic JSON to tolerate format changes.
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return fmt.Errorf("parse recent paths JSON: %w", err)
	}

	entriesRaw, ok := data["entries"]
	if !ok {
		return fmt.Errorf("no entries key in recent paths")
	}

	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(entriesRaw, &entries); err != nil {
		return fmt.Errorf("parse entries array: %w", err)
	}

	// Build the URI we're looking for.
	targetURI := pathToFileURI(folderPath)

	filtered := make([]map[string]json.RawMessage, 0, len(entries))
	removed := 0
	for _, entry := range entries {
		if matchesPath(entry, targetURI, folderPath) {
			removed++
			continue
		}
		filtered = append(filtered, entry)
	}

	if removed == 0 {
		return nil // nothing to do
	}

	// Marshal back.
	newEntries, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("marshal filtered entries: %w", err)
	}
	data["entries"] = newEntries

	newJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal recent paths: %w", err)
	}

	_, err = db.Exec("UPDATE ItemTable SET value = ? WHERE key = ?", string(newJSON), key)
	if err != nil {
		return fmt.Errorf("write recent paths: %w", err)
	}

	return nil
}

// matchesPath checks whether a recently-opened entry refers to the given path.
// Handles both folderUri entries and workspace entries whose configPath is under the folder.
func matchesPath(entry map[string]json.RawMessage, targetURI, targetPath string) bool {
	// Check folderUri field.
	if raw, ok := entry["folderUri"]; ok {
		var uri string
		if json.Unmarshal(raw, &uri) == nil && uri == targetURI {
			return true
		}
	}

	// Check workspace.configPath field.
	if raw, ok := entry["workspace"]; ok {
		var ws map[string]string
		if json.Unmarshal(raw, &ws) == nil {
			if cp, ok := ws["configPath"]; ok {
				if decoded := fileURIToPath(cp); decoded != "" {
					if strings.HasPrefix(decoded, targetPath+"/") {
						return true
					}
				}
			}
		}
	}

	return false
}

// pathToFileURI converts an absolute path to a file:// URI.
func pathToFileURI(path string) string {
	return "file://" + path
}

// fileURIToPath extracts the path from a file:// URI, returning "" on failure.
func fileURIToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	return u.Path
}
