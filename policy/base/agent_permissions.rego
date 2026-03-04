# Atomic OS - Base Agent Permissions Policy
# Package: atomic.agent
#
# This is the core safety contract for AI agents running on Agentic OS.
# The fundamental principle is DENY BY DEFAULT: every agent action is denied
# unless at least one rule explicitly allows it AND no deny rule fires.
#
# Policy evaluation flow:
#   1. atomicagentd receives an action request from an agent
#   2. The OPA engine evaluates this file against the agent's active profile
#   3. If allow=true and no deny rule fires, the action proceeds
#   4. All decisions are written to the audit log

package atomic.agent

import future.keywords.in
import future.keywords.if
import future.keywords.every

# --- Core decision outputs ---

# Default: deny everything
default allow := false
default audit_required := false
default reasons := []

# An action is allowed if at least one allow rule fires
# AND no deny rule fires (deny takes precedence)
allow if {
	some_allow_rule_matches
	not deny_rule_matches
}

# Deny overrides allow for sensitive operations
deny_rule_matches if {
	action_targets_sensitive_path
}

deny_rule_matches if {
	input.action.action_type == "exec"
	not input.action.resource in data.atomic.agent.allowed_binaries
}

# Audit is required for any infrastructure-level action
audit_required if {
	input.agent.profile == "infrastructure"
}

# Audit is required for all network operations
audit_required if {
	input.action.action_type == "network_connect"
}

# Audit is required for all exec operations
audit_required if {
	input.action.action_type == "exec"
}

# --- Allow rules ---

# Allow filesystem reads within the agent's own workspace
some_allow_rule_matches if {
	input.action.action_type == "filesystem_read"
	startswith(input.action.resource, input.agent.workspace_path)
}

# Allow filesystem writes within the agent's own workspace
some_allow_rule_matches if {
	input.action.action_type == "filesystem_write"
	startswith(input.action.resource, input.agent.workspace_path)
}

# Allow reads of the agent's own policy (read-only view of what it can do)
some_allow_rule_matches if {
	input.action.action_type == "filesystem_read"
	startswith(input.action.resource, "/usr/share/atomic/policy")
}

# Allow reading shared runtime info
some_allow_rule_matches if {
	input.action.action_type == "filesystem_read"
	input.action.resource in {"/etc/hostname", "/etc/os-release", "/proc/cpuinfo", "/proc/meminfo"}
}

# Allow tool_call actions (these are mediated by the agent runtime, not the OS)
# Tool call authorization is handled separately by the profile policies
some_allow_rule_matches if {
	input.action.action_type == "tool_call"
}

# --- Deny rules for sensitive paths ---

sensitive_paths := {
	"/etc/shadow",
	"/etc/passwd",
	"/etc/gshadow",
	"/root",
	"/proc/sysrq-trigger",
}

sensitive_prefixes := [
	"/etc/ssh",
	"/root/",
	"/home",
	"/run/atomic",   # daemon sockets - agents cannot talk to the daemon directly
	"/var/lib/atomic/agents",  # other agents' workspaces
	"/boot",
	"/usr/lib/systemd",
	"/usr/bin/atomicagentd",
	"/usr/bin/atomic-agent-ctl",
]

action_targets_sensitive_path if {
	input.action.action_type in {"filesystem_write", "filesystem_delete"}
	input.action.resource in sensitive_paths
}

action_targets_sensitive_path if {
	input.action.action_type in {"filesystem_read", "filesystem_write", "filesystem_delete"}
	some prefix in sensitive_prefixes
	startswith(input.action.resource, prefix)
}

# Agents cannot access other agents' workspaces
action_targets_sensitive_path if {
	input.action.action_type in {"filesystem_read", "filesystem_write", "filesystem_delete"}
	startswith(input.action.resource, "/var/lib/atomic/agents/")
	not startswith(input.action.resource, input.agent.workspace_path)
}

# --- Default allowed binaries (exec allowlist) ---
# Profiles can extend this list. It cannot be narrowed by profiles (only expanded).
allowed_binaries := {
	"/usr/bin/python3",
	"/usr/bin/python",
	"/usr/bin/node",
	"/usr/bin/bash",
	"/usr/bin/sh",
	"/usr/bin/curl",
	"/usr/bin/git",
	"/usr/bin/env",
}
