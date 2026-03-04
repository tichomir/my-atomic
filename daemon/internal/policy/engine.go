package policy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/open-policy-agent/opa/v1/rego"
)

// Engine evaluates agent actions against OPA Rego policies.
// All evaluations are deny-by-default: actions must be explicitly allowed.
type Engine struct {
	mu      sync.RWMutex
	queries map[string]rego.PreparedEvalQuery // keyed by profile name
	agents  map[string]*AgentRecord
	dirs    []string // policy directories to load from
}

// NewEngine creates a policy engine and loads policies from the given directories.
// builtinDir is read-only (/usr/share/atomic/policy).
// overrideDir is operator-mutable (/etc/atomic/policy) and may not exist.
func NewEngine(builtinDir, overrideDir string) (*Engine, error) {
	e := &Engine{
		queries: make(map[string]rego.PreparedEvalQuery),
		agents:  make(map[string]*AgentRecord),
	}

	dirs := []string{builtinDir}
	if overrideDir != "" {
		if _, err := os.Stat(overrideDir); err == nil {
			dirs = append(dirs, overrideDir)
		}
	}
	e.dirs = dirs

	if err := e.loadPolicies(); err != nil {
		return nil, fmt.Errorf("loading policies: %w", err)
	}

	return e, nil
}

// loadPolicies reads all .rego files from policy directories and prepares
// evaluation queries for each profile.
func (e *Engine) loadPolicies() error {
	profiles := []string{"minimal", "developer", "infrastructure"}

	for _, profile := range profiles {
		query, err := e.prepareQuery(profile)
		if err != nil {
			return fmt.Errorf("preparing query for profile %s: %w", profile, err)
		}
		e.queries[profile] = query
	}

	return nil
}

// prepareQuery builds an OPA PreparedEvalQuery for a given profile.
// It loads base policies plus the profile-specific overlay.
func (e *Engine) prepareQuery(profile string) (rego.PreparedEvalQuery, error) {
	var modules []func(*rego.Rego)

	for _, dir := range e.dirs {
		// Load base policies
		baseDir := filepath.Join(dir, "base")
		if files, err := filepath.Glob(filepath.Join(baseDir, "*.rego")); err == nil {
			for _, f := range files {
				data, err := os.ReadFile(f)
				if err != nil {
					return rego.PreparedEvalQuery{}, fmt.Errorf("reading %s: %w", f, err)
				}
				modules = append(modules, rego.Module(f, string(data)))
			}
		}

		// Load profile-specific policy overlay
		profileFile := filepath.Join(dir, "profiles", profile+".rego")
		if data, err := os.ReadFile(profileFile); err == nil {
			modules = append(modules, rego.Module(profileFile, string(data)))
		}
	}

	if len(modules) == 0 {
		return rego.PreparedEvalQuery{}, fmt.Errorf("no policy files found in %v", e.dirs)
	}

	regoOpts := append([]func(*rego.Rego){
		rego.Query("data.atomic.agent"),
	}, modules...)

	return rego.New(regoOpts...).PrepareForEval(context.Background())
}

// Evaluate checks whether an agent action is permitted under its active profile.
// This is the hot path - called for every action an agent attempts.
func (e *Engine) Evaluate(ctx context.Context, action AgentAction) (PolicyDecision, error) {
	e.mu.RLock()
	agent, ok := e.agents[action.AgentID]
	if !ok {
		e.mu.RUnlock()
		// Unknown agents are denied by default
		return PolicyDecision{
			Allow:   false,
			Reasons: []string{"agent not registered"},
			Profile: "none",
		}, nil
	}
	profile := agent.Profile
	query, queryOK := e.queries[profile]
	e.mu.RUnlock()

	if !queryOK {
		return PolicyDecision{
			Allow:   false,
			Reasons: []string{fmt.Sprintf("unknown policy profile: %s", profile)},
			Profile: profile,
		}, nil
	}

	input := OPAInput{
		Action: action,
		Agent:  *agent,
	}

	rs, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return PolicyDecision{Allow: false, Profile: profile}, fmt.Errorf("policy evaluation: %w", err)
	}

	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return PolicyDecision{
			Allow:   false,
			Reasons: []string{"no policy result"},
			Profile: profile,
		}, nil
	}

	// Extract allow and audit_required from result
	result, ok := rs[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		return PolicyDecision{Allow: false, Profile: profile}, fmt.Errorf("unexpected policy result type")
	}

	allow, _ := result["allow"].(bool)
	auditRequired, _ := result["audit_required"].(bool)

	var reasons []string
	if rawReasons, ok := result["reasons"].([]interface{}); ok {
		for _, r := range rawReasons {
			if s, ok := r.(string); ok {
				reasons = append(reasons, s)
			}
		}
	}

	return PolicyDecision{
		Allow:         allow,
		AuditRequired: auditRequired,
		Reasons:       reasons,
		Profile:       profile,
	}, nil
}

// RegisterAgent adds an agent to the engine's registry.
func (e *Engine) RegisterAgent(agent *AgentRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agents[agent.ID] = agent
}

// UnregisterAgent removes an agent from the registry.
func (e *Engine) UnregisterAgent(agentID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.agents, agentID)
}

// GetAgent returns the AgentRecord for the given ID.
func (e *Engine) GetAgent(agentID string) (*AgentRecord, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	a, ok := e.agents[agentID]
	return a, ok
}

// Reload hot-reloads all policy files without restarting the daemon.
func (e *Engine) Reload() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	newQueries := make(map[string]rego.PreparedEvalQuery)
	profiles := []string{"minimal", "developer", "infrastructure"}

	for _, profile := range profiles {
		query, err := e.prepareQuery(profile)
		if err != nil {
			return fmt.Errorf("reloading profile %s: %w", profile, err)
		}
		newQueries[profile] = query
	}

	e.queries = newQueries
	return nil
}

// ValidateDir checks that all .rego files in a directory are syntactically valid.
// Returns an error describing any invalid files.
func ValidateDir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "**", "*.rego"))
	if err != nil {
		return err
	}
	// Also check root level
	rootFiles, _ := filepath.Glob(filepath.Join(dir, "*.rego"))
	files = append(files, rootFiles...)

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}
		_, err = rego.New(
			rego.Query("true"),
			rego.Module(f, string(data)),
		).PrepareForEval(context.Background())
		if err != nil {
			return fmt.Errorf("invalid policy %s: %w", f, err)
		}
	}
	return nil
}
