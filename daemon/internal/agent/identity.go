package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var validAgentIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{0,62}[a-z0-9]$`)

// ValidateID checks that an agent ID is safe for use as a systemd unit name
// and as a filesystem path component.
func ValidateID(id string) error {
	if len(id) < 2 || len(id) > 64 {
		return fmt.Errorf("agent ID must be 2-64 characters, got %d", len(id))
	}
	if !validAgentIDPattern.MatchString(id) {
		return fmt.Errorf("agent ID must match [a-z0-9][a-z0-9-]{0,62}[a-z0-9], got %q", id)
	}
	// Prevent path traversal
	if strings.Contains(id, "..") || strings.Contains(id, "/") {
		return fmt.Errorf("agent ID must not contain path separators")
	}
	return nil
}

// GenerateToken creates a cryptographically random token for agent authentication.
// Tokens are used to authenticate agent API calls to atomicagentd.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// WorkspacePath returns the canonical path for an agent's workspace directory.
func WorkspacePath(workspaceRoot, agentID string) string {
	return fmt.Sprintf("%s/%s/workspace", workspaceRoot, agentID)
}

// SystemdUnitName returns the systemd unit name for an agent.
func SystemdUnitName(agentID string) string {
	return fmt.Sprintf("atomic-agent@%s.service", agentID)
}
