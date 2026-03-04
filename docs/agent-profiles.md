# Agent Profiles

## Overview

Every agent registered with Agentic OS has an assigned **profile** that governs its permissions. Profiles are evaluated at both the policy layer (OPA) and the runtime layer (systemd sandbox limits).

## Profile Summary

### `minimal` — Default

For untrusted, exploratory, or single-purpose agents.

| Capability | Value |
|---|---|
| Network | None (private network namespace) |
| Exec | None |
| Filesystem | Workspace read/write only |
| Memory | 256 MB |
| CPU | 50% |
| Tasks | 32 |
| Workspace quota | 1 GB |

**When to use**: New agents, agents processing user-provided data, any agent where you are not certain of its behavior.

### `developer`

For CI/CD pipelines, development tooling, code generation agents.

| Capability | Value |
|---|---|
| Network | Public internet (private ranges blocked, metadata blocked) |
| Exec | Development tools (npm, pip, git, docker, kubectl, terraform...) |
| Filesystem | Workspace read/write + standard system reads |
| Memory | 1 GB |
| CPU | 150% |
| Tasks | 128 |
| Workspace quota | 10 GB |

**When to use**: Agents that need to build and test code, agents that call public APIs.

### `infrastructure`

For trusted automation agents managing production systems. **Requires explicit operator decision to assign.**

| Capability | Value |
|---|---|
| Network | Full (private ranges allowed; metadata service ALWAYS blocked) |
| Exec | Full toolchain including cloud CLIs and system tools |
| Filesystem | Workspace + read-only /etc inspection |
| Memory | 2 GB |
| CPU | 200% |
| Tasks | 256 |
| Workspace quota | 50 GB |
| Audit | ALL actions force-audited |

**When to use**: Agents managing cloud resources, running Ansible playbooks, operating Kubernetes clusters. All actions are audit-logged.

## Assigning a Profile

```bash
# At registration time
atomic-agent-ctl agent register my-agent --profile developer --exec /usr/bin/my-agent

# Check current profile
atomic-agent-ctl agent status my-agent
```

Changing a profile requires stopping and re-registering the agent.

## Custom Profiles

Custom profiles can be added by dropping a `.rego` file in `/etc/atomic/policy/profiles/` and reloading:

```bash
sudo nano /etc/atomic/policy/profiles/my-custom-profile.rego
atomic-agent-ctl policy reload
```

A custom profile must be in `package atomic.agent.profiles.<name>` and define at minimum:
- `profile_name`
- `network_allowed`
- `exec_allowed`
