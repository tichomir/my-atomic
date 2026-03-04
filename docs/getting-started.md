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

```bash
# Register a minimal (most restrictive) agent
atomic-agent-ctl agent register hello-agent \
  --profile minimal \
  --exec /usr/bin/python3

# Start it
atomic-agent-ctl agent start hello-agent

# Check its status
atomic-agent-ctl agent status hello-agent

# Test a policy decision
atomic-agent-ctl policy eval hello-agent \
  --action filesystem_write \
  --resource /var/lib/atomic/agents/hello-agent/workspace/hello.txt

# Watch audit events
journalctl -t atomic-audit -f -o json | jq '{type:.type, agent:.agent_id, decision:.decision}'

# Stop the agent
atomic-agent-ctl agent stop hello-agent
```

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
