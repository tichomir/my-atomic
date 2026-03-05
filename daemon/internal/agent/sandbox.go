package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	godbus "github.com/godbus/dbus/v5"

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
//
// All hardening properties are set here rather than relying on a static
// template unit, because StartTransientUnit fails if any fragment file
// (including a template like atomic-agent@.service) already exists for
// the unit name. Setting properties via D-Bus also works within the daemon's
// own ProtectSystem=strict sandbox since no filesystem writes are required.
func (m *SandboxManager) StartAgent(ctx context.Context, cfg SandboxConfig) error {
	unitName := SystemdUnitName(cfg.AgentID)

	// Build systemd unit properties for the transient unit.
	// Property.Value must be a godbus.Variant (github.com/godbus/dbus/v5).
	properties := []dbus.Property{
		dbus.PropDescription(fmt.Sprintf("Atomic Agent: %s [%s]", cfg.AgentID, cfg.Profile)),
		dbus.PropExecStart([]string{cfg.ExecStart}, false),

		// Resource limits (per-profile values override these via cfg)
		{Name: "MemoryMax", Value: makeVariant(uint64(cfg.MemoryMaxMB) * 1024 * 1024)},
		{Name: "CPUQuotaPerSecUSec", Value: makeVariant(uint64(cfg.CPUQuotaPct) * 10000)},
		{Name: "TasksMax", Value: makeVariant(uint64(cfg.TasksMax))},
		{Name: "LimitNOFILE", Value: makeVariant(uint64(1024))},

		// Workspace and environment
		{Name: "BindPaths", Value: makeVariant([]string{cfg.WorkspacePath})},
		{Name: "Environment", Value: makeVariant([]string{
			fmt.Sprintf("ATOMIC_AGENT_ID=%s", cfg.AgentID),
			fmt.Sprintf("ATOMIC_AGENT_PROFILE=%s", cfg.Profile),
			"ATOMIC_POLICY_SOCKET=/run/atomic/policy.sock",
			"ATOMIC_AUDIT_SOCKET=/run/atomic/audit.sock",
		})},

		// Identity: ephemeral UID per agent, no persistent identity
		{Name: "DynamicUser", Value: makeVariant(true)},

		// Network isolation
		{Name: "PrivateNetwork", Value: makeVariant(!cfg.AllowNetwork)},
		// RestrictAddressFamilies D-Bus type is (bas): bool (is-allowlist) + []string (families)
		{Name: "RestrictAddressFamilies", Value: makeVariant(addressFamilyRestriction{
			IsAllowList: true,
			Families:    []string{"AF_UNIX", "AF_INET", "AF_INET6"},
		})},

		// Filesystem hardening
		{Name: "PrivateTmp", Value: makeVariant(true)},
		{Name: "PrivateDevices", Value: makeVariant(true)},
		{Name: "PrivateIPC", Value: makeVariant(true)},
		{Name: "ProtectSystem", Value: makeVariant("strict")},
		// ProtectHome D-Bus type is string "yes"/"no"/"read-only"/"tmpfs", not bool
		{Name: "ProtectHome", Value: makeVariant("yes")},
		{Name: "ProtectKernelTunables", Value: makeVariant(true)},
		{Name: "ProtectKernelModules", Value: makeVariant(true)},
		{Name: "ProtectClock", Value: makeVariant(true)},
		{Name: "ProtectHostname", Value: makeVariant(true)},
		{Name: "ProtectProc", Value: makeVariant("invisible")},

		// Privilege hardening
		{Name: "CapabilityBoundingSet", Value: makeVariant(uint64(0))},
		{Name: "NoNewPrivileges", Value: makeVariant(true)},
		{Name: "MemoryDenyWriteExecute", Value: makeVariant(true)},
		{Name: "RestrictSUIDSGID", Value: makeVariant(true)},
		{Name: "LockPersonality", Value: makeVariant(true)},
		{Name: "RestrictRealtime", Value: makeVariant(true)},
	}

	ch := make(chan string)
	_, err := m.conn.StartTransientUnitContext(ctx, unitName, "fail", properties, ch)
	if err != nil {
		return fmt.Errorf("starting transient unit %s: %w", unitName, err)
	}

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
	// Use KillUnitWithTarget (returns error) instead of deprecated KillUnitContext (returns nothing).
	return m.conn.KillUnitWithTarget(context.Background(), unitName, dbus.All, 9)
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

// addressFamilyRestriction is the Go representation of the D-Bus (bas) struct
// that systemd expects for the RestrictAddressFamilies property.
// IsAllowList=true means the Families list is an allowlist; false means denylist.
type addressFamilyRestriction struct {
	IsAllowList bool
	Families    []string
}

// makeVariant wraps a value in godbus.Variant, which is the type required by
// go-systemd's dbus.Property.Value field.
func makeVariant(v interface{}) godbus.Variant {
	return godbus.MakeVariant(v)
}
