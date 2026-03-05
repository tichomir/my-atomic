# Agentic OS Architecture

## Overview

Agentic OS provides three interlocking safety layers for AI agents:

1. **Policy Engine** (OPA/Rego) — evaluates every agent action before it executes
2. **Sandbox Isolation** (systemd units) — enforces OS-level process isolation
3. **Runtime Detection** (Falco/eBPF) — monitors kernel events for violations

These layers are independent and mutually reinforcing. Bypassing one does not bypass the others.

## The Agent Action Lifecycle

```
Agent wants to perform an action (e.g., write a file)
          │
          ▼
atomicagentd receives action request via policy socket
          │
          ▼
┌─────────────────────────────┐
│  OPA Policy Engine          │
│  Evaluates:                 │
│  - action type              │
│  - resource path            │
│  - agent profile            │
│  - deny rules               │
│                             │
│  Result: ALLOW / DENY       │
└────────────┬────────────────┘
             │
      ┌──────┴──────┐
   ALLOW           DENY
      │              │
      ▼              ▼
Action proceeds   Action blocked
Audit log: ALLOW  Audit log: DENY
      │
      ▼
Falco watches the syscall (independently)
  - If violation detected: KILL + CRITICAL audit event
```

## Sandbox Model

Every agent runs as a transient systemd unit created via D-Bus by `atomicagentd`.
All hardening properties are set programmatically in the `StartTransientUnit` call
(not from a static unit file), which works within `atomicagentd`'s own
`ProtectSystem=strict` sandbox without requiring filesystem writes.

| Property | Value | Effect |
|---|---|---|
| `DynamicUser=yes` | ephemeral UID | No persistent identity across runs |
| `CapabilityBoundingSet=` | empty | Zero Linux capabilities |
| `NoNewPrivileges=yes` | enforced | setuid binaries are ineffective |
| `PrivateTmp=yes` | isolated | Agent cannot see other /tmp files |
| `PrivateDevices=yes` | isolated | No /dev access |
| `PrivateIPC=yes` | isolated | No shared memory / IPC access |
| `PrivateNetwork=yes` | minimal profile | Complete network isolation |
| `ProtectSystem=strict` | read-only | /usr, /boot are immutable to the agent |
| `ProtectHome=yes` | enforced | No access to user home directories |
| `ProtectProc=invisible` | enforced | Agent cannot see other processes in /proc |
| `ProtectKernelTunables=yes` | enforced | No sysctl writes |
| `ProtectKernelModules=yes` | enforced | No module loading |
| `ProtectClock=yes` | enforced | No system clock manipulation |
| `ProtectHostname=yes` | enforced | Cannot change hostname |
| `MemoryDenyWriteExecute=yes` | enforced | No JIT injection |
| `RestrictSUIDSGID=yes` | enforced | setuid/setgid bits are ignored |
| `LockPersonality=yes` | enforced | Cannot change execution domain |
| `RestrictRealtime=yes` | enforced | No real-time scheduling |
| `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6` | allowlist | No raw sockets, no netlink |
| `BindPaths=workspace` | explicit | Only the agent's workspace is writable |

## Policy Engine

Policies are written in [OPA Rego](https://www.openpolicyagent.org/docs/latest/policy-language/).

**Evaluation is deny-by-default**: an action is only allowed if `allow=true` AND `deny_rule_matches=false`.

Policy files live in two locations (bootc semantics):
- `/usr/share/atomic/policy/` — immutable, baked into the OCI image
- `/etc/atomic/policy/` — operator-overridable, survives `bootc upgrade`

Hot-reload without restart: `atomic-agent-ctl policy reload`

## Audit Trail

Every decision produces a JSON event written to the systemd journal with `SYSLOG_IDENTIFIER=atomic-audit`.

Query examples:
```bash
# All audit events
journalctl -t atomic-audit -o json

# Only policy denials
journalctl -t atomic-audit -o json | jq 'select(.type == "policy.deny")'

# Events for a specific agent
journalctl -t atomic-audit -o json | jq 'select(.agent_id == "my-agent")'

# Critical runtime alerts
journalctl -t atomic-audit -o json | jq 'select(.severity == "CRITICAL")'
```

## OS Immutability (bootc + Hummingbird)

Atomic Linux uses two complementary Red Hat projects for OS management:

- **bootc** handles deployment, updates, and rollback — the OS is an OCI image
- **Project Hummingbird** provides the minimal, zero-CVE base image content that Atomic Linux builds from

Together they give Atomic OS both a minimal attack surface (Hummingbird) and atomic update semantics (bootc).

### bootc mechanics

- `/usr` is mounted read-only via composefs
- OS updates are OCI image pulls: `bootc upgrade`
- Rollback is instant: `bootc rollback`
- Config in `/etc` persists across updates (operator overrides)
- State in `/var` persists across updates (agent workspaces)

This means an agent cannot modify OS binaries even if it somehow escapes its sandbox — the filesystem is physically read-only at the kernel level.

## Threat Model

**What Agentic OS protects against:**

| Threat | Protection |
|---|---|
| Agent reading /etc/shadow | OPA deny rule + Falco CRITICAL rule |
| Prompt injection → shell spawn | Falco rule (shell spawn from agent process) |
| Agent installing system packages | Falco rule (package manager spawn) |
| SSRF to cloud metadata | OPA network deny + Falco CRITICAL rule + iptables |
| Agent escaping to other agent workspace | OPA deny rule + Falco cross-workspace rule |
| Agent modifying OS binaries | composefs read-only /usr |
| Agent gaining root | DynamicUser + empty CapabilityBoundingSet |

**What Agentic OS does NOT protect against:**

- Vulnerabilities in the agent runtime itself (e.g., a Python CVE)
- Agents that are given infrastructure-profile and misuse it (operator trust decision)
- Physical access to the machine
- Kernel 0-days (though lockdown=integrity raises the bar)
