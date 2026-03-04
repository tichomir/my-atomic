# Policy Reference

## Policy Language

Agentic OS uses [OPA Rego](https://www.openpolicyagent.org/docs/latest/policy-language/) for policy authoring. All policies are in `policy/`.

## Input Document Structure

Every policy evaluation receives an `input` document with this structure:

```json
{
  "action": {
    "agent_id": "my-agent",
    "action_type": "filesystem_write",
    "resource": "/var/lib/atomic/agents/my-agent/workspace/output.txt",
    "context": {},
    "timestamp": "2026-03-04T12:00:00Z"
  },
  "agent": {
    "id": "my-agent",
    "profile": "developer",
    "workspace_path": "/var/lib/atomic/agents/my-agent/workspace",
    "allowed_binaries": [],
    "allowed_networks": []
  }
}
```

## Action Types

| Action Type | Description |
|---|---|
| `filesystem_read` | Read a file or directory |
| `filesystem_write` | Write or create a file |
| `filesystem_delete` | Delete a file or directory |
| `network_connect` | Establish a network connection (resource = IP or hostname) |
| `exec` | Execute a binary (resource = full binary path) |
| `tool_call` | Call an external tool or API |
| `syscall` | Direct syscall interception (advanced) |

## Policy Outputs

A policy must produce these outputs in `package atomic.agent`:

| Output | Type | Description |
|---|---|---|
| `allow` | boolean | Whether the action is permitted (default: `false`) |
| `audit_required` | boolean | Whether to force audit this action even if allowed |
| `reasons` | array of strings | Human-readable explanation of the decision |

## Writing a Custom Policy

```rego
package atomic.agent

import future.keywords.if
import future.keywords.in

# Allow agents to read from a shared read-only data directory
allow if {
    input.action.action_type == "filesystem_read"
    startswith(input.action.resource, "/var/lib/atomic/shared-data/")
}

# Always audit reads to shared data
audit_required if {
    input.action.action_type == "filesystem_read"
    startswith(input.action.resource, "/var/lib/atomic/shared-data/")
}
```

Place this file in `/etc/atomic/policy/base/my_policy.rego` and run `atomic-agent-ctl policy reload`.

## Testing Policies

```bash
# Run unit tests
opa test policy/

# Test a specific input interactively
echo '{
  "action": {"action_type": "filesystem_read", "resource": "/etc/shadow"},
  "agent": {"id": "test", "profile": "minimal", "workspace_path": "/var/lib/atomic/agents/test/workspace"}
}' | opa eval -I -d policy/ 'data.atomic.agent.allow'
# false (correctly denied)
```

## Profile Hierarchy

Profiles are additive. The `base/` policies define the minimum floor that all profiles inherit. Profile-specific files in `profiles/` can only expand permissions, not reduce them below the base floor.

```
base/agent_permissions.rego  ← always evaluated
base/network_policy.rego     ← always evaluated
base/filesystem_policy.rego  ← always evaluated
        +
profiles/minimal.rego        ← OR developer.rego OR infrastructure.rego
```

## Sensitive Path Reference

These paths are always denied for writes regardless of profile:

- `/etc/**` — system configuration
- `/usr/**` — system binaries and libraries (also read-only via composefs)
- `/boot/**` — bootloader
- `/run/atomic/**` — atomicagentd runtime sockets
- `/proc/**`, `/sys/**`, `/dev/**` — kernel interfaces

These paths are always denied for reads:

- `/etc/shadow` — password hashes
- `/etc/gshadow` — group password hashes
- `/root/.ssh/**` — root SSH keys
- `/home/*/.ssh/**` — user SSH keys
- `/home/*/.aws/**` — AWS credentials
- `/home/*/.gcloud/**` — GCP credentials
