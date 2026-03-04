package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coreos/go-systemd/v22/dbus"
)

// SandboxConfig holds the parameters for an agent's systemd sandbox unit.
type SandboxConfig struct {
	AgentID       string
	ExecStart     string
	WorkspacePath string
	Profile       string
	MemoryMaxMB   int
	CPUQuotaPct   int
	TasksMax      int
	AllowNetwork  bool
}

// DefaultSandboxConfig returns conservative defaults for an agent sandbox.
func DefaultSandboxConfig(agentID, execStart, workspacePath, profile string) SandboxConfig {
	return SandboxConfig{
		AgentID:       agentID,
		ExecStart:     execStart,
		WorkspacePath: workspacePath,
		Profile:       profile,
		MemoryMaxMB:   512,
		CPUQuotaPct:   100,
		TasksMax:      64,
		AllowNetwork:  false,
	}
}

// SandboxManager manages agent systemd transient units.
type SandboxManager struct {
	conn          *dbus.Conn
	workspaceRoot string
}

// NewSandboxManager creates a SandboxManager connected to the system bus.
func NewSandboxManager(workspaceRoot string) (*SandboxManager, error) {
	conn, err := dbus.NewSystemConnectionContext(context.Background())
	if err != nil {
		return nil, fmt.Errorf("connecting to systemd D-Bus: %w", err)
	}
	return &SandboxManager{conn: conn, workspaceRoot: workspaceRoot}, nil
}

// ProvisionWorkspace creates the agent's workspace directory.
func (m *SandboxManager) ProvisionWorkspace(agentID string) (string, error) {
	path := WorkspacePath(m.workspaceRoot, agentID)
	if err := os.MkdirAll(path, 0750); err != nil {
		return "", fmt.Errorf("creating workspace %s: %w", path, err)
	}
	// Create a metadata file so the workspace is identifiable
	meta := filepath.Join(m.workspaceRoot, agentID, "agent.id")
	if err := os.WriteFile(meta, []byte(agentID), 0640); err != nil {
		return "", fmt.Errorf("writing agent metadata: %w", err)
	}
	return path, nil
}

// StartAgent starts a sandboxed agent as a transient systemd unit.
func (m *SandboxManager) StartAgent(ctx context.Context, cfg SandboxConfig) error {
	unitName := SystemdUnitName(cfg.AgentID)

	// Build systemd unit properties for the transient unit.
	// These apply the hardening from the template unit plus profile-specific limits.
	properties := []dbus.Property{
		dbus.PropDescription(fmt.Sprintf("Atomic Agent: %s [%s]", cfg.AgentID, cfg.Profile)),
		dbus.PropExecStart([]string{cfg.ExecStart}, false),
		// Inherit hardening from the template but apply profile resource limits
		{Name: "MemoryMax", Value: dbusVariant(uint64(cfg.MemoryMaxMB) * 1024 * 1024)},
		{Name: "CPUQuotaPerSecUSec", Value: dbusVariant(uint64(cfg.CPUQuotaPct) * 10000)},
		{Name: "TasksMax", Value: dbusVariant(uint64(cfg.TasksMax))},
		// Workspace bind mount
		{Name: "BindPaths", Value: dbusVariant([]string{cfg.WorkspacePath})},
		// Environment
		{Name: "Environment", Value: dbusVariant([]string{
			fmt.Sprintf("ATOMIC_AGENT_ID=%s", cfg.AgentID),
			fmt.Sprintf("ATOMIC_AGENT_PROFILE=%s", cfg.Profile),
			"ATOMIC_POLICY_SOCKET=/run/atomic/policy.sock",
			"ATOMIC_AUDIT_SOCKET=/run/atomic/audit.sock",
		})},
		// Network restriction for minimal profile
		{Name: "PrivateNetwork", Value: dbusVariant(!cfg.AllowNetwork)},
		// Dynamic user for ephemeral UID isolation
		{Name: "DynamicUser", Value: dbusVariant(true)},
	}

	ch := make(chan string)
	_, err := m.conn.StartTransientUnitContext(ctx, unitName, "fail", properties, ch)
	if err != nil {
		return fmt.Errorf("starting transient unit %s: %w", unitName, err)
	}

	// Wait for the job to complete
	result := <-ch
	if result != "done" {
		return fmt.Errorf("systemd job for %s finished with status: %s", unitName, result)
	}

	return nil
}

// StopAgent stops a running agent unit gracefully.
func (m *SandboxManager) StopAgent(ctx context.Context, agentID string) error {
	unitName := SystemdUnitName(agentID)
	ch := make(chan string)
	_, err := m.conn.StopUnitContext(ctx, unitName, "replace", ch)
	if err != nil {
		return fmt.Errorf("stopping unit %s: %w", unitName, err)
	}
	<-ch
	return nil
}

// KillAgent immediately sends SIGKILL to an agent.
func (m *SandboxManager) KillAgent(agentID string) error {
	unitName := SystemdUnitName(agentID)
	return m.conn.KillUnitContext(context.Background(), unitName, int32(9))
}

// AgentStatus returns the current systemd unit status for an agent.
func (m *SandboxManager) AgentStatus(agentID string) (string, error) {
	unitName := SystemdUnitName(agentID)
	props, err := m.conn.GetUnitPropertiesContext(context.Background(), unitName)
	if err != nil {
		return "", fmt.Errorf("getting properties for %s: %w", unitName, err)
	}
	state, ok := props["ActiveState"].(string)
	if !ok {
		return "unknown", nil
	}
	return state, nil
}

// Close releases the D-Bus connection.
func (m *SandboxManager) Close() {
	m.conn.Close()
}

// dbusVariant wraps a value in the dbus.Variant type required by go-systemd.
func dbusVariant(v interface{}) dbus.Variant {
	return dbus.MakeVariant(v)
}
