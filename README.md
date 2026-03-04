# Atomic OS

Atomic OS is a free, open-source OS platform for safely running AI agents in infrastructure.

It combines two complementary Red Hat initiatives:

- **[Project Hummingbird](https://www.redhat.com/en/blog/red-hat-hummingbird)** — Red Hat's zero-CVE minimal container image project. Provides the hardened, minimal base images that Atomic Linux builds on. Think of it as Red Hat's answer to Alpine/Wolfi: small attack surface, no unnecessary packages, security-first.
- **[bootc](https://github.com/containers/bootc)** — Bootable Containers project. Treats the entire OS as an OCI image that can be deployed, updated, and rolled back like a container pull.

Atomic OS uses Hummingbird images as the trusted, minimal content layer and `bootc` as the deployment/update mechanism.

| Component | Description |
|---|---|
| **Atomic Linux** | Free, minimal, bootable OS built from Hummingbird base images, managed via `bootc` |
| **Atomic Images** | The OCI images produced by this project — bootable via `bootc`, minimal via Hummingbird principles |
| **Agentic OS** | AI agent safety runtime: sandboxing, policy engine, and threat detection |

The core motivation: running an AI agent in your infrastructure without safety guardrails is dangerous. The agent will install software, reach unexpected network destinations, and access files outside its intended scope. Agentic OS enforces these boundaries at the OS level.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                  AGENTIC OS                         │
│  atomicagentd  |  OPA policy  |  Falco detection   │
│  systemd sandboxes  |  audit log  |  agent CLI      │
├─────────────────────────────────────────────────────┤
│               ATOMIC LINUX (base)                   │
│  fedora-bootc  |  kernel LTS  |  SELinux  |  audit  │
├─────────────────────────────────────────────────────┤
│    BOOTC (OS deployment) + HUMMINGBIRD (content)    │
│  bootc  |  ostree  |  composefs  |  zero-CVE images │
└─────────────────────────────────────────────────────┘
```

## Key Safety Features

- **Deny-by-default policy engine** (OPA/Rego): every agent action requires an explicit allow rule
- **systemd sandbox isolation**: each agent runs with an ephemeral UID, no capabilities, private filesystem
- **Falco runtime detection**: kernel-level behavioral monitoring with agent-aware rules
- **Tamper-proof audit log**: all agent actions written as structured JSON to systemd journal
- **Auto-kill on critical violations**: atomicagentd kills agents automatically on CRITICAL Falco alerts
- **Immutable OS base**: `/usr` is read-only (bootc composefs), agents cannot modify system files
- **Atomic updates**: OS updates via `bootc switch` or `bootc upgrade` with instant rollback

## Quick Start

### Install on a bootc-capable system

```bash
# Switch an existing Fedora/RHEL bootc system to Agentic OS
sudo bootc switch ghcr.io/tichomir/agentic-os:latest
sudo reboot
```

### Register and run an AI agent

```bash
# Register an agent with the minimal (most restrictive) profile
atomic-agent-ctl agent register my-agent \
  --profile minimal \
  --exec /usr/bin/my-ai-agent

# Start the agent in its sandbox
atomic-agent-ctl agent start my-agent

# Check status
atomic-agent-ctl system status

# Watch audit events in real time
journalctl -t atomic-audit -f -o json | jq .
```

### Test the policy engine

```bash
# Test if an agent action would be allowed
atomic-agent-ctl policy eval my-agent \
  --action filesystem_write \
  --resource /var/lib/atomic/agents/my-agent/workspace/output.txt
# [ALLOW] agent=my-agent action=filesystem_write ...

# This will be denied
atomic-agent-ctl policy eval my-agent \
  --action filesystem_read \
  --resource /etc/shadow
# [DENY] agent=my-agent action=filesystem_read ...
```

## Policy Profiles

| Profile | Network | Exec | Workspace | Use Case |
|---|---|---|---|---|
| `minimal` | No | No | 1 GB | Untrusted/exploratory agents |
| `developer` | Public internet | Yes | 10 GB | CI/CD, dev tooling |
| `infrastructure` | Full (no metadata) | Yes | 50 GB | Trusted automation (requires explicit operator auth) |

## Free-to-Fee Model

Atomic OS is free and freely redistributable. Commercial tiers add:
- Compliance policy profiles (SOC2, HIPAA, PCI-DSS)
- Red Hat signed builds with CVE SLAs
- Enterprise audit integrations (Splunk, Datadog)
- Fleet management console

## Build

See [docs/getting-started.md](docs/getting-started.md) for build prerequisites.

```bash
cd build
make test          # Run all tests
make image-agentic # Build the full agentic-os OCI image
make qcow2         # Build a QEMU disk image for local testing
```

## Repository Structure

```
my-atomic/
├── images/              # Bootable OCI image definitions (Containerfiles)
│   ├── atomic-base/     # Atomic Linux base image
│   ├── atomic-cloud/    # Cloud-provider additions
│   └── agentic-os/      # Full agentic safety stack
├── os/                  # OS-layer config (kernel args, systemd units, sysctl)
├── daemon/              # Go: atomicagentd daemon + atomic-agent-ctl CLI
├── policy/              # OPA Rego policies (deny-by-default)
│   ├── base/            # Core permission rules
│   ├── profiles/        # minimal, developer, infrastructure
│   └── tests/           # Policy unit tests
├── runtime/             # Falco configuration and agent detection rules
├── build/               # Makefile and build scripts
└── docs/                # Architecture, policy reference, getting started
```

## License

MIT. See [LICENSE](LICENSE).
