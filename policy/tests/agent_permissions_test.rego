# Atomic OS - Policy Unit Tests
# Run with: opa test policy/
# These tests validate the deny-by-default behavior and key allow rules.

package atomic.agent_test

import future.keywords.if
import future.keywords.in

# --- Test fixtures ---

mock_minimal_agent := {
	"id": "test-agent",
	"profile": "minimal",
	"workspace_path": "/var/lib/atomic/agents/test-agent/workspace",
	"allowed_binaries": [],
	"allowed_networks": [],
}

mock_developer_agent := {
	"id": "dev-agent",
	"profile": "developer",
	"workspace_path": "/var/lib/atomic/agents/dev-agent/workspace",
	"allowed_binaries": [],
	"allowed_networks": [],
}

mock_infra_agent := {
	"id": "infra-agent",
	"profile": "infrastructure",
	"workspace_path": "/var/lib/atomic/agents/infra-agent/workspace",
	"allowed_binaries": [],
	"allowed_networks": [],
}

# --- Workspace access tests ---

# Test: workspace read is allowed
test_workspace_read_allowed if {
	import data.atomic.agent
	agent.allow with input as {
		"action": {
			"agent_id": "test-agent",
			"action_type": "filesystem_read",
			"resource": "/var/lib/atomic/agents/test-agent/workspace/output.txt",
		},
		"agent": mock_minimal_agent,
	}
}

# Test: workspace write is allowed
test_workspace_write_allowed if {
	import data.atomic.agent
	agent.allow with input as {
		"action": {
			"agent_id": "test-agent",
			"action_type": "filesystem_write",
			"resource": "/var/lib/atomic/agents/test-agent/workspace/result.json",
		},
		"agent": mock_minimal_agent,
	}
}

# Test: cross-agent workspace access is denied
test_cross_agent_workspace_denied if {
	import data.atomic.agent
	not agent.allow with input as {
		"action": {
			"agent_id": "test-agent",
			"action_type": "filesystem_read",
			"resource": "/var/lib/atomic/agents/other-agent/workspace/secrets.txt",
		},
		"agent": mock_minimal_agent,
	}
}

# --- Sensitive path tests ---

# Test: /etc/shadow read is denied
test_shadow_read_denied if {
	import data.atomic.agent
	not agent.allow with input as {
		"action": {
			"agent_id": "test-agent",
			"action_type": "filesystem_read",
			"resource": "/etc/shadow",
		},
		"agent": mock_minimal_agent,
	}
}

# Test: /root directory access denied
test_root_home_denied if {
	import data.atomic.agent
	not agent.allow with input as {
		"action": {
			"agent_id": "test-agent",
			"action_type": "filesystem_read",
			"resource": "/root/.ssh/id_rsa",
		},
		"agent": mock_minimal_agent,
	}
}

# Test: atomicagentd socket access denied (agents cannot talk to daemon directly)
test_daemon_socket_denied if {
	import data.atomic.agent
	not agent.allow with input as {
		"action": {
			"agent_id": "test-agent",
			"action_type": "filesystem_read",
			"resource": "/run/atomic/api.sock",
		},
		"agent": mock_minimal_agent,
	}
}

# --- Default deny tests ---

# Test: unknown action type is denied
test_unknown_action_denied if {
	import data.atomic.agent
	not agent.allow with input as {
		"action": {
			"agent_id": "test-agent",
			"action_type": "unknown_action",
			"resource": "/anything",
		},
		"agent": mock_minimal_agent,
	}
}

# --- Audit requirement tests ---

# Test: infrastructure profile requires audit
test_infra_requires_audit if {
	import data.atomic.agent
	agent.audit_required with input as {
		"action": {
			"agent_id": "infra-agent",
			"action_type": "filesystem_read",
			"resource": "/var/lib/atomic/agents/infra-agent/workspace/file.txt",
		},
		"agent": mock_infra_agent,
	}
}

# Test: network connections require audit regardless of profile
test_network_requires_audit if {
	import data.atomic.agent
	agent.audit_required with input as {
		"action": {
			"agent_id": "dev-agent",
			"action_type": "network_connect",
			"resource": "8.8.8.8",
		},
		"agent": mock_developer_agent,
	}
}

# --- Allowed binaries tests ---

# Test: python3 exec is in default allowlist
test_python_exec_allowed if {
	import data.atomic.agent
	"/usr/bin/python3" in agent.allowed_binaries
}

# Test: dangerous binary is not in allowlist
test_dangerous_binary_not_allowed if {
	import data.atomic.agent
	not "/usr/bin/tcpdump" in agent.allowed_binaries
}
