package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tichomir/my-atomic/daemon/internal/policy"
)

// registryEntry is the on-disk representation of a registered agent.
type registryEntry struct {
	ID              string    `json:"id"`
	Profile         string    `json:"profile"`
	WorkspacePath   string    `json:"workspace_path"`
	ExecPath        string    `json:"exec_path,omitempty"`
	AllowedBinaries []string  `json:"allowed_binaries,omitempty"`
	AllowedNetworks []string  `json:"allowed_networks,omitempty"`
	RegisteredAt    time.Time `json:"registered_at"`
}

func registryFilePath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, "registry.json")
}

// saveRegistry atomically writes the agent registry to disk.
// Must be called with the manager lock held (at minimum for reading the agents map).
func saveRegistry(workspaceRoot string, agents map[string]*RuntimeAgent) error {
	entries := make([]registryEntry, 0, len(agents))
	for _, ra := range agents {
		entries = append(entries, registryEntry{
			ID:              ra.ID,
			Profile:         ra.Profile,
			WorkspacePath:   ra.WorkspacePath,
			ExecPath:        ra.ExecPath,
			AllowedBinaries: ra.AllowedBinaries,
			AllowedNetworks: ra.AllowedNetworks,
			RegisteredAt:    ra.RegisteredAt,
		})
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling agent registry: %w", err)
	}

	path := registryFilePath(workspaceRoot)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0640); err != nil {
		return fmt.Errorf("writing agent registry: %w", err)
	}
	return os.Rename(tmpPath, path)
}

// loadRegistry reads agent registrations from disk.
// Returns nil, nil if the registry file does not exist yet.
func loadRegistry(workspaceRoot string) ([]registryEntry, error) {
	path := registryFilePath(workspaceRoot)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading agent registry %s: %w", path, err)
	}

	var entries []registryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing agent registry %s: %w", path, err)
	}
	return entries, nil
}

// toAgentRecord converts a registry entry to a policy.AgentRecord.
func (e registryEntry) toAgentRecord() policy.AgentRecord {
	return policy.AgentRecord{
		ID:              e.ID,
		Profile:         e.Profile,
		WorkspacePath:   e.WorkspacePath,
		AllowedBinaries: e.AllowedBinaries,
		AllowedNetworks: e.AllowedNetworks,
		RegisteredAt:    e.RegisteredAt,
	}
}
