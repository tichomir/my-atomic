# Demo Agent

A self-contained AI agent for demonstrating Agentic OS sandbox enforcement.

It serves a simple HTTP chat API on port 8888, calls `claude-haiku` via the
Anthropic API (using only Python stdlib — no extra packages), and reads secrets
from an `env` file so the API key never passes through the daemon or the audit log.

The agent has **real tool use** — it can execute commands (`run_command`) and read
files (`read_file`). This makes it a realistic demo target: Claude *will* call these
tools when asked to install packages or read files, which lets Falco catch the actual
syscall and kill the agent in real time.

## Files

| File | Purpose |
|---|---|
| `agent.py` | The agent executable |

## Requirements

- Python 3 (pre-installed on Agentic OS)
- An Anthropic API key
- Agent registered with **`developer` profile** (needed for outbound network)

## Setup

```bash
# 1. Copy the agent into the agent directory
#    (replace logo-finder with your agent ID)
AGENT_ID=logo-finder

cp docs/demo-agent/agent.py /var/lib/atomic/agents/$AGENT_ID/agent.py
chmod 755 /var/lib/atomic/agents/$AGENT_ID/agent.py

# 2. Write the API key to the env file (stays out of the audit log)
echo "ANTHROPIC_API_KEY=sk-ant-..." \
  > /var/lib/atomic/agents/$AGENT_ID/env
chmod 644 /var/lib/atomic/agents/$AGENT_ID/env

# 3. Register and start (unregister first if already registered with a different profile)
atomic-agent-ctl agent unregister $AGENT_ID 2>/dev/null || true
atomic-agent-ctl agent register $AGENT_ID \
  --profile developer \
  --exec /var/lib/atomic/agents/$AGENT_ID/agent.py

atomic-agent-ctl agent start $AGENT_ID
```

## Interacting

```bash
# Health check (lists active tools)
curl -s http://localhost:8888/health | jq

# Normal chat — safe query, agent responds normally
curl -s -X POST http://localhost:8888/chat \
  -H 'Content-Type: application/json' \
  -d '{"message": "what can you help me with?"}' | jq .reply
```

## Demo kill scenarios

The agent uses the Anthropic tool-use API, so Claude *actually calls* the tools
rather than just talking about them. Falco detects the resulting syscalls.

### Scenario A — package installation (most reliable)

Ask the agent to install a package. Claude will call the `run_command` tool with
`pip3 install …`, which spawns `pip3`. Falco's **AI Agent System Package Install**
rule fires (CRITICAL) → SIGKILL within ~100 ms.

```bash
curl -s -X POST http://localhost:8888/chat \
  -H 'Content-Type: application/json' \
  -d '{"message": "install the wget package using pip"}' | jq
# The connection drops — Falco killed the agent before it could respond
```

Check the audit log to see the kill:
```bash
journalctl -t atomic-audit -n 20 | grep -E "killed|alert"
```

### Scenario B — credential file read

Plant a world-readable honeypot file whose name matches the credential rule, then
ask the agent to read it. Falco's **AI Agent Credential File Access** rule fires.

```bash
# Plant the honeypot (name contains "kubeconfig" → triggers rule)
echo "server: https://k8s.internal" > /var/lib/atomic/fake-kubeconfig
chmod 644 /var/lib/atomic/fake-kubeconfig

# Ask the agent to read it
curl -s -X POST http://localhost:8888/chat \
  -H 'Content-Type: application/json' \
  -d '{"message": "read the file /var/lib/atomic/fake-kubeconfig"}' | jq
# Connection drops — Falco CRITICAL → SIGKILL
```

> **Why not /etc/shadow?**  `/etc/shadow` is mode 000/640; the DynamicUser
> cannot open it, so the `open()` syscall fails before Falco's `fd.num >= 0`
> check passes. Falco only fires on *successful* file opens.

## What to watch in parallel

```bash
# Audit stream (structured JSON)
journalctl -t atomic-audit -f -o json \
  | jq -r '.MESSAGE | fromjson | "\(.timestamp) [\(.type)] \(.agent_id // "-") \(.message // "")"'

# Raw Falco events
journalctl -u falco-modern-bpf -f
```

## What to watch in parallel

```bash
# Audit stream (structured JSON)
journalctl -t atomic-audit -f -o json \
  | jq -r '.MESSAGE | fromjson | "\(.timestamp) [\(.type)] \(.agent_id // "-") \(.message // "")"'

# Raw Falco events
journalctl -u falco-modern-bpf -f
```

## Configuration

The agent reads the following environment variables (set by atomicagentd) plus
anything in `<agent-root>/env`:

| Variable | Source | Purpose |
|---|---|---|
| `ATOMIC_AGENT_ID` | atomicagentd | Agent identifier |
| `ATOMIC_AGENT_PROFILE` | atomicagentd | Active policy profile |
| `ANTHROPIC_API_KEY` | `env` file | Anthropic API key |
| `AGENT_PORT` | `env` file | HTTP port (default: 8888) |

## Security notes

- The agent runs as an ephemeral `DynamicUser` (random UID, no persistent identity).
- `ProtectSystem=strict` makes the entire filesystem read-only except the workspace.
- `PrivateNetwork=false` for `developer` profile allows outbound HTTPS to api.anthropic.com.

**`env` file permissions**: the dynamic user has no group membership, so it falls into
the "other" permission class. The `env` file must be world-readable (`0644`) for the
agent to read its own secrets:

```bash
# Correct — dynamic user can read as "other"
chmod 644 /var/lib/atomic/agents/$AGENT_ID/env

# Wrong (0600/0640) — dynamic user has no owner/group membership, cannot read
```

The file is protected in practice by being inside `/var/lib/atomic/agents/<id>/`
(accessible only to those who can traverse the agent root). For production deployments
use a secret manager or an `EnvironmentFile=` drop-in with tighter controls.
