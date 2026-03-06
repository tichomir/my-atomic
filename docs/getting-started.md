# Getting Started with Agentic OS

## Prerequisites

### To build images

- `podman` 4.x or later
- `go` 1.24 or later
- `opa` CLI (for policy tests)
- A container registry account (GitHub Packages / quay.io)

### To run Agentic OS

- A VM or bare-metal host capable of running Fedora bootc
- Or: use `bootc-image-builder` to create a QEMU image locally

## Building locally

```bash
git clone https://github.com/tichomir/my-atomic
cd my-atomic

# Run all tests first
cd build && make test

# Build the base atomic-linux image
make image-base

# Build the full agentic-os image
make image-agentic
```

## Testing with QEMU

```bash
cd build

# Build a QEMU disk image (requires root for bootc-image-builder)
make qcow2

# Launch the VM
make run-vm
# SSH: ssh -p 2222 cloud-user@localhost
```

## Deploying to an existing bootc system

If you already have a Fedora bootc or RHEL image-mode system:

```bash
# Switch to Agentic OS
sudo bootc switch ghcr.io/tichomir/agentic-os:latest

# Reboot to activate
sudo systemctl reboot

# After reboot, verify
systemctl status atomicagentd
atomic-agent-ctl system status
```

## Your first agent

Each agent needs an executable script in its own directory under
`/var/lib/atomic/agents/`. The directory is created automatically by
`atomicagentd` when you register the agent. The script must be
world-readable and world-executable so the sandboxed dynamic user can run it.

```bash
# 1. Register the agent — atomicagentd creates /var/lib/atomic/agents/hello-agent/
atomic-agent-ctl agent register hello-agent \
  --profile minimal \
  --exec /var/lib/atomic/agents/hello-agent/run.py

# 2. Place and permission the exec script
cat > /var/lib/atomic/agents/hello-agent/run.py << 'EOF'
#!/usr/bin/python3
import os, time
agent_id = os.environ.get("ATOMIC_AGENT_ID", "unknown")
workspace = f"/var/lib/atomic/agents/{agent_id}/workspace"
print(f"[{agent_id}] started, workspace={workspace}", flush=True)
# Write a file to the workspace to prove it works
with open(f"{workspace}/hello.txt", "w") as f:
    f.write("hello from the sandbox\n")
print(f"[{agent_id}] wrote hello.txt", flush=True)
time.sleep(3600)   # stay alive so you can inspect it
EOF
chmod 755 /var/lib/atomic/agents/hello-agent/run.py

# 3. Start it
atomic-agent-ctl agent start hello-agent

# 4. Check its status
atomic-agent-ctl agent status hello-agent

# 5. Verify the workspace write landed
cat /var/lib/atomic/agents/hello-agent/workspace/hello.txt

# 6. Test a policy decision
atomic-agent-ctl policy eval hello-agent \
  --action filesystem_write \
  --resource /var/lib/atomic/agents/hello-agent/workspace/hello.txt
# [ALLOW]

atomic-agent-ctl policy eval hello-agent \
  --action filesystem_read \
  --resource /etc/shadow
# [DENY]

# 7. Watch audit events
journalctl -t atomic-audit -f -o json | jq '{type:.type, agent:.agent_id, decision:.decision}'

# 8. Stop the agent
atomic-agent-ctl agent stop hello-agent
```

### Agent directory layout

```
/var/lib/atomic/agents/<id>/
├── run.py         # your exec script (world-readable, +x)
├── env            # optional secrets file (chmod 600, read at startup)
│                  # format: KEY=value, one per line, # for comments
└── workspace/     # writable by the agent, bind-mounted into the sandbox
```

> **Secrets**: pass API keys via the `env` file rather than hardcoding them.
> `atomicagentd` does not pass env vars through the registration API, so the
> agent script reads `/var/lib/atomic/agents/<id>/env` at startup. See
> `docs/demo-agent/agent.py` for an example.

## Upgrading the OS

```bash
# Pull and stage the latest Agentic OS image
sudo bootc upgrade

# Apply on next reboot (non-disruptive)
sudo systemctl reboot

# Or: apply immediately
sudo bootc upgrade --apply
```

## Rolling back

```bash
# If something goes wrong after an upgrade:
sudo bootc rollback
sudo systemctl reboot
```

## Customizing policies

Operator policy overrides go in `/etc/atomic/policy/` and survive OS upgrades.

```bash
# Create an operator-specific policy override
sudo mkdir -p /etc/atomic/policy/base
sudo nano /etc/atomic/policy/base/my_overrides.rego

# Hot-reload without restarting atomicagentd
atomic-agent-ctl policy reload
```

See [docs/policy-reference.md](policy-reference.md) for the policy authoring guide.
