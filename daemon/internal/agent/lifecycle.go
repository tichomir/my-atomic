package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tichomir/my-atomic/daemon/internal/audit"
	"github.com/tichomir/my-atomic/daemon/internal/policy"
)

// State represents the current lifecycle state of an agent.
type State string

const (
	StateRegistered State = "registered"
	StateStarting   State = "starting"
	StateRunning    State = "running"
	StateStopping   State = "stopping"
	StateStopped    State = "stopped"
	StateKilled     State = "killed"
	StateFailed     State = "failed"
)

// RuntimeAgent combines the policy record with live runtime state.
type RuntimeAgent struct {
	policy.AgentRecord
	State         State
	StartedAt     *time.Time
	StoppedAt     *time.Time
	ExitCode      *int
	KillReason    string
}

// Manager orchestrates the full lifecycle of agents: registration, start, stop, kill.
type Manager struct {
	mu        sync.RWMutex
	agents    map[string]*RuntimeAgent
	sandbox   *SandboxManager
	policyEng *policy.Engine
	auditor   *audit.Logger
	cfg       ManagerConfig
}

// ManagerConfig holds configuration for the Manager.
type ManagerConfig struct {
	WorkspaceRoot string
	MaxAgents     int
}

// NewManager creates an agent Manager.
func NewManager(cfg ManagerConfig, sandbox *SandboxManager, eng *policy.Engine, auditor *audit.Logger) *Manager {
	return &Manager{
		agents:    make(map[string]*RuntimeAgent),
		sandbox:   sandbox,
		policyEng: eng,
		auditor:   auditor,
		cfg:       cfg,
	}
}

// Register adds a new agent to the registry without starting it.
func (m *Manager) Register(id, profile, execStart string) (*RuntimeAgent, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.agents[id]; exists {
		return nil, fmt.Errorf("agent %q is already registered", id)
	}

	if m.cfg.MaxAgents > 0 && len(m.agents) >= m.cfg.MaxAgents {
		return nil, fmt.Errorf("maximum number of agents (%d) reached", m.cfg.MaxAgents)
	}

	workspacePath, err := m.sandbox.ProvisionWorkspace(id)
	if err != nil {
		return nil, fmt.Errorf("provisioning workspace for %s: %w", id, err)
	}

	now := time.Now().UTC()
	record := policy.AgentRecord{
		ID:           id,
		Profile:      profile,
		WorkspacePath: workspacePath,
		RegisteredAt: now,
	}

	ra := &RuntimeAgent{
		AgentRecord: record,
		State:       StateRegistered,
	}

	m.agents[id] = ra
	m.policyEng.RegisterAgent(&record)

	m.auditor.LogAgentLifecycle(id, audit.EventAgentRegistered,
		fmt.Sprintf("agent %s registered with profile %s", id, profile))

	return ra, nil
}

// Start launches a registered agent inside its sandbox.
func (m *Manager) Start(ctx context.Context, id string) error {
	m.mu.Lock()
	ra, ok := m.agents[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("agent %q not registered", id)
	}
	if ra.State == StateRunning || ra.State == StateStarting {
		m.mu.Unlock()
		return fmt.Errorf("agent %q is already %s", id, ra.State)
	}
	ra.State = StateStarting
	execStart := ra.ExecStart()
	workspacePath := ra.WorkspacePath
	profile := ra.Profile
	m.mu.Unlock()

	sandboxCfg := DefaultSandboxConfig(id, execStart, workspacePath, profile)

	// Infrastructure and developer profiles get network access
	if profile == "infrastructure" || profile == "developer" {
		sandboxCfg.AllowNetwork = true
	}

	// Profile-based resource limits
	switch profile {
	case "infrastructure":
		sandboxCfg.MemoryMaxMB = 2048
		sandboxCfg.CPUQuotaPct = 200
		sandboxCfg.TasksMax = 256
	case "developer":
		sandboxCfg.MemoryMaxMB = 1024
		sandboxCfg.CPUQuotaPct = 150
		sandboxCfg.TasksMax = 128
	default: // minimal
		sandboxCfg.MemoryMaxMB = 256
		sandboxCfg.CPUQuotaPct = 50
		sandboxCfg.TasksMax = 32
	}

	if err := m.sandbox.StartAgent(ctx, sandboxCfg); err != nil {
		m.mu.Lock()
		ra.State = StateFailed
		m.mu.Unlock()
		return fmt.Errorf("starting agent sandbox: %w", err)
	}

	now := time.Now().UTC()
	m.mu.Lock()
	ra.State = StateRunning
	ra.StartedAt = &now
	ra.AgentRecord.LastStartedAt = &now
	m.mu.Unlock()

	m.auditor.LogAgentLifecycle(id, audit.EventAgentStarted,
		fmt.Sprintf("agent %s started with profile %s", id, profile))

	return nil
}

// Stop gracefully stops a running agent.
func (m *Manager) Stop(ctx context.Context, id string) error {
	m.mu.Lock()
	ra, ok := m.agents[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("agent %q not found", id)
	}
	ra.State = StateStopping
	m.mu.Unlock()

	if err := m.sandbox.StopAgent(ctx, id); err != nil {
		return fmt.Errorf("stopping agent %s: %w", id, err)
	}

	now := time.Now().UTC()
	m.mu.Lock()
	ra.State = StateStopped
	ra.StoppedAt = &now
	m.mu.Unlock()

	m.auditor.LogAgentLifecycle(id, audit.EventAgentStopped,
		fmt.Sprintf("agent %s stopped", id))

	return nil
}

// Kill immediately terminates an agent (SIGKILL). Used by runtime alerts.
func (m *Manager) Kill(id, reason string) error {
	if err := m.sandbox.KillAgent(id); err != nil {
		return fmt.Errorf("killing agent %s: %w", id, err)
	}

	now := time.Now().UTC()
	m.mu.Lock()
	if ra, ok := m.agents[id]; ok {
		ra.State = StateKilled
		ra.StoppedAt = &now
		ra.KillReason = reason
	}
	m.mu.Unlock()

	m.auditor.LogAgentLifecycle(id, audit.EventAgentKilled,
		fmt.Sprintf("agent %s killed: %s", id, reason))

	return nil
}

// GetStatus returns the current state of an agent.
func (m *Manager) GetStatus(id string) (*RuntimeAgent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ra, ok := m.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent %q not found", id)
	}
	// Return a copy
	copy := *ra
	return &copy, nil
}

// ListAgents returns all registered agents.
func (m *Manager) ListAgents() []*RuntimeAgent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*RuntimeAgent, 0, len(m.agents))
	for _, ra := range m.agents {
		copy := *ra
		result = append(result, &copy)
	}
	return result
}

// ExecStart returns the agent's executable path.
// The actual exec path is stored in the AgentRecord context.
func (ra *RuntimeAgent) ExecStart() string {
	if path, ok := ra.AgentRecord.AllowedBinaries[0:1]; ok && len(path) > 0 {
		return path[0]
	}
	return "/bin/false" // safe default - agent will fail to start
}
