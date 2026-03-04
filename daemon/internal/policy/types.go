package policy

import "time"

// AgentAction represents a single action an AI agent wants to perform.
// The policy engine evaluates every action before it is permitted.
type AgentAction struct {
	// AgentID is the unique identifier of the agent requesting the action
	AgentID string `json:"agent_id"`

	// ActionType classifies the action category
	// Valid values: "filesystem_read", "filesystem_write", "filesystem_delete",
	//               "network_connect", "exec", "tool_call", "syscall"
	ActionType string `json:"action_type"`

	// Resource is the target of the action (path, URL, binary, tool name)
	Resource string `json:"resource"`

	// Context provides additional metadata about the action
	Context map[string]string `json:"context,omitempty"`

	// Timestamp records when the action was requested
	Timestamp time.Time `json:"timestamp"`
}

// PolicyDecision is the result of evaluating an AgentAction against policies.
type PolicyDecision struct {
	// Allow indicates whether the action is permitted
	Allow bool `json:"allow"`

	// Reasons explains why the decision was made (from Rego rules)
	Reasons []string `json:"reasons,omitempty"`

	// AuditRequired indicates this action must be logged even if allowed
	AuditRequired bool `json:"audit_required"`

	// Profile is the active policy profile used for evaluation
	Profile string `json:"profile"`
}

// AgentRecord holds persistent state about a registered agent.
type AgentRecord struct {
	// ID is the unique agent identifier
	ID string `json:"id"`

	// Profile is the policy profile name (minimal, developer, infrastructure)
	Profile string `json:"profile"`

	// WorkspacePath is the agent's writable directory
	WorkspacePath string `json:"workspace_path"`

	// AllowedBinaries overrides the profile's exec allowlist
	AllowedBinaries []string `json:"allowed_binaries,omitempty"`

	// AllowedNetworks is a list of CIDR ranges the agent may connect to
	AllowedNetworks []string `json:"allowed_networks,omitempty"`

	// RegisteredAt records when the agent was registered
	RegisteredAt time.Time `json:"registered_at"`

	// LastStartedAt records the most recent start time
	LastStartedAt *time.Time `json:"last_started_at,omitempty"`
}

// OPAInput is the full input document passed to OPA for evaluation.
// Matches the structure expected by policy/base/agent_permissions.rego.
type OPAInput struct {
	Action AgentAction            `json:"action"`
	Agent  AgentRecord            `json:"agent"`
	Data   map[string]interface{} `json:"data,omitempty"`
}
