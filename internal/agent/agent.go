// Package agent manages per-worktree agent registration, discovery, and status.
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// AgentEntry describes a running worktree agent.
type AgentEntry struct {
	Port         int       `json:"port"`
	PID          int       `json:"pid"`
	WorktreePath string    `json:"worktree_path"`
	Branch       string    `json:"branch"`
	Project      string    `json:"project"`
	StartTime    time.Time `json:"start_time"`
}

// Registry holds all known agents keyed by "{project}/{task}".
type Registry struct {
	Agents map[string]AgentEntry `json:"agents"`
}

// StatusResponse is returned by each agent's /status endpoint.
type StatusResponse struct {
	Branch        string `json:"branch"`
	LastCommit    string `json:"last_commit"`
	ClaudeRunning bool   `json:"claude_running"`
	Uptime        string `json:"uptime"`
}

// RegistryPath returns the path to agents.json, creating the parent dir if needed.
func RegistryPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	dir := filepath.Join(configDir, "task-plus")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("mkdir config: %w", err)
	}
	return filepath.Join(dir, "agents.json"), nil
}

// lockPath returns the flock sibling path.
func lockPath(regPath string) string {
	return regPath + ".lock"
}

// withLock runs fn while holding an exclusive flock on agents.json.lock.
func withLock(fn func() error) error {
	regPath, err := RegistryPath()
	if err != nil {
		return err
	}
	lp := lockPath(regPath)
	f, err := os.OpenFile(lp, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("open lock: %w", err)
	}
	defer f.Close() //nolint:errcheck // best-effort cleanup

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("flock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	return fn()
}

// Load reads the registry from disk. Returns an empty registry if the file doesn't exist.
func Load() (*Registry, error) {
	regPath, err := RegistryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(regPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Registry{Agents: make(map[string]AgentEntry)}, nil
		}
		return nil, fmt.Errorf("read registry: %w", err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	if reg.Agents == nil {
		reg.Agents = make(map[string]AgentEntry)
	}
	return &reg, nil
}

// Save writes the registry atomically (write tmp, rename).
func Save(reg *Registry) error {
	regPath, err := RegistryPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	tmp := regPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, regPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// Register upserts an agent entry under flock.
func Register(key string, entry AgentEntry) error {
	return withLock(func() error {
		reg, err := Load()
		if err != nil {
			return err
		}
		reg.Agents[key] = entry
		return Save(reg)
	})
}

// Deregister removes an agent entry under flock.
func Deregister(key string) error {
	return withLock(func() error {
		reg, err := Load()
		if err != nil {
			return err
		}
		delete(reg.Agents, key)
		return Save(reg)
	})
}

// CleanStale removes entries whose PID is no longer alive. Returns removed keys.
func CleanStale() ([]string, error) {
	var removed []string
	err := withLock(func() error {
		reg, err := Load()
		if err != nil {
			return err
		}
		for key, entry := range reg.Agents {
			if !processAlive(entry.PID) {
				delete(reg.Agents, key)
				removed = append(removed, key)
			}
		}
		if len(removed) > 0 {
			return Save(reg)
		}
		return nil
	})
	return removed, err
}

// processAlive checks if a PID is still running via kill(pid, 0).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
