package audit

import "time"

// EventType classifies what kind of audit event occurred.
type EventType string

const (
	EventAgentRegistered   EventType = "agent.registered"
	EventAgentStarted      EventType = "agent.started"
	EventAgentStopped      EventType = "agent.stopped"
	EventAgentKilled       EventType = "agent.killed"
	EventPolicyAllow       EventType = "policy.allow"
	EventPolicyDeny        EventType = "policy.deny"
	EventPolicyReloaded    EventType = "policy.reloaded"
	EventRuntimeAlert      EventType = "runtime.alert"
	EventRuntimeCritical   EventType = "runtime.critical"
	EventDaemonStarted     EventType = "daemon.started"
	EventDaemonShutdown    EventType = "daemon.shutdown"
)

// Severity maps to syslog severity levels.
type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityError    Severity = "ERROR"
	SeverityCritical Severity = "CRITICAL"
)

// Event is a structured audit log entry.
// All fields are serialized to JSON for journald and file output.
type Event struct {
	// Timestamp in RFC3339Nano format
	Timestamp time.Time `json:"timestamp"`

	// EventType classifies the event
	Type EventType `json:"type"`

	// Severity level
	Severity Severity `json:"severity"`

	// AgentID is the agent involved (empty for daemon-level events)
	AgentID string `json:"agent_id,omitempty"`

	// ActionType is the action the agent attempted (for policy events)
	ActionType string `json:"action_type,omitempty"`

	// Resource is the resource the agent targeted
	Resource string `json:"resource,omitempty"`

	// PolicyProfile is the active policy profile for the agent
	PolicyProfile string `json:"policy_profile,omitempty"`

	// Decision is "allow" or "deny" for policy events
	Decision string `json:"decision,omitempty"`

	// Reasons explains the policy decision
	Reasons []string `json:"reasons,omitempty"`

	// Message is a human-readable description
	Message string `json:"message"`

	// FalcoRule is set for runtime.alert events
	FalcoRule string `json:"falco_rule,omitempty"`

	// FalcoOutput is the raw Falco alert output
	FalcoOutput string `json:"falco_output,omitempty"`

	// PID of the agent process (for runtime events)
	PID int `json:"pid,omitempty"`
}

// NewEvent creates an Event with the current timestamp.
func NewEvent(eventType EventType, severity Severity, message string) Event {
	return Event{
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		Severity:  severity,
		Message:   message,
	}
}
