package audit

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// Logger writes structured audit events to systemd journal and/or a file.
// Journal writes use the SYSLOG_IDENTIFIER=atomic-audit field so that
// events can be queried with: journalctl -t atomic-audit -o json
type Logger struct {
	mu         sync.Mutex
	fileWriter *os.File
	fileEnabled bool
	journalEnabled bool
	slogger    *slog.Logger
}

// Config configures the audit logger.
type Config struct {
	JournalEnabled     bool
	JournalIdentifier  string
	FileEnabled        bool
	FilePath           string
}

// NewLogger creates a new audit Logger.
func NewLogger(cfg Config) (*Logger, error) {
	l := &Logger{
		journalEnabled: cfg.JournalEnabled,
		fileEnabled:    cfg.FileEnabled,
	}

	// slog writes to stderr which systemd captures as journal entries.
	// The SYSLOG_IDENTIFIER is set by the service unit.
	l.slogger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if cfg.FileEnabled && cfg.FilePath != "" {
		f, err := os.OpenFile(cfg.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, fmt.Errorf("opening audit file %s: %w", cfg.FilePath, err)
		}
		l.fileWriter = f
	}

	return l, nil
}

// Log records an audit event.
func (l *Logger) Log(event Event) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		l.slogger.Error("failed to marshal audit event", "error", err)
		return
	}

	if l.journalEnabled {
		// Write to stderr as structured JSON; systemd journal captures it.
		// Using fmt.Fprintf directly preserves exact JSON structure.
		fmt.Fprintf(os.Stderr, "%s\n", data)
	}

	if l.fileEnabled && l.fileWriter != nil {
		fmt.Fprintf(l.fileWriter, "%s\n", data)
	}
}

// LogAgentAction logs a policy decision for an agent action.
func (l *Logger) LogAgentAction(agentID, actionType, resource, profile, decision string, reasons []string) {
	severity := SeverityInfo
	eventType := EventPolicyAllow
	if decision == "deny" {
		severity = SeverityWarning
		eventType = EventPolicyDeny
	}

	e := NewEvent(eventType, severity, fmt.Sprintf("agent %s %s on %s: %s", agentID, actionType, resource, decision))
	e.AgentID = agentID
	e.ActionType = actionType
	e.Resource = resource
	e.PolicyProfile = profile
	e.Decision = decision
	e.Reasons = reasons
	l.Log(e)
}

// LogRuntimeAlert logs a Falco alert.
func (l *Logger) LogRuntimeAlert(agentID, rule, output string, critical bool) {
	severity := SeverityWarning
	eventType := EventRuntimeAlert
	if critical {
		severity = SeverityCritical
		eventType = EventRuntimeCritical
	}

	e := NewEvent(eventType, severity, fmt.Sprintf("runtime alert for agent %s: %s", agentID, rule))
	e.AgentID = agentID
	e.FalcoRule = rule
	e.FalcoOutput = output
	l.Log(e)
}

// LogAgentLifecycle logs agent start/stop/kill events.
func (l *Logger) LogAgentLifecycle(agentID string, eventType EventType, message string) {
	e := NewEvent(eventType, SeverityInfo, message)
	e.AgentID = agentID
	l.Log(e)
}

// Close flushes and closes the file writer if open.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fileWriter != nil {
		return l.fileWriter.Close()
	}
	return nil
}
