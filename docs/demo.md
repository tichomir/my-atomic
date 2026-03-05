# Agentic OS Demo Guide

This guide walks through a complete demo of Agentic OS from scratch.
It covers three scenarios, ordered from simplest to most compelling.

---

## Prerequisites

You need one of the following:

| Option | What you need |
|---|---|
| **Local VM (recommended)** | macOS or Linux with QEMU/UTM, 4 GB RAM, 20 GB disk |
| **Cloud VM** | AWS/GCP/Azure instance (Fedora 42 or RHEL 10) |
| **Existing bootc system** | Any system already running `fedora-bootc` or RHEL image mode |

You also need on your **build machine** (your Mac):
- `podman` — `brew install podman`
- `go` 1.24+ — `brew install go`
- `opa` — `brew install opa`

---

## Part 1: Build and Boot the OS (15 minutes)

### Step 1 — Clone and build

```bash
git clone https://github.com/tichomir/my-atomic
cd my-atomic

# Generate go.sum (needed once)
cd daemon && go mod tidy && cd ..

# Run all tests
cd build && make test
# Expected: daemon tests pass, OPA policy tests pass

# Build the agentic-os OCI image
make image-agentic
# This runs three Containerfile builds stacked:
#   fedora-bootc:42 → atomic-linux → atomic-cloud → agentic-os
```

### Step 2 — Convert to a bootable disk image

```bash
# macOS only: podman machine is not needed on Linux (podman runs natively there)
podman machine init --cpus 2 --memory 4096 --disk-size 30 --rootful
podman machine start
podman machine ssh -- sudo usermod -aG wheel core

# Build the QEMU disk image
make qcow2
# Output: build/output/qcow2/disk.qcow2
```

### Step 3 — Boot the VM

```bash
# Launch with QEMU (macOS)
qemu-system-x86_64 \
  -m 2048 \
  -smp 2 \
  -drive file=build/output/qcow2/disk.qcow2,if=virtio \
  -net user,hostfwd=tcp::2222-:22 \
  -net nic \
  -nographic

# Or use UTM on macOS (import disk.qcow2 as a new VM)
```

### Step 4 — SSH in and verify

```bash
ssh -p 2222 cloud-user@localhost

# Verify Agentic OS is running
$ systemctl status atomicagentd
● atomicagentd.service - Atomic Agent Daemon
     Loaded: loaded (/usr/lib/systemd/system/atomicagentd.service; enabled; preset: enabled)
     Active: active (running)

$ atomic-agent-ctl system status
atomicagentd: ok
Agents: 0 total, 0 running
```

---

## Part 2: The Core Demo — Agent Safety in Action (10 minutes)

This reproduces the exact scenario from the All Hands meeting: an agent that installs software on your machine without asking permission.

### Scenario A — The unsafe agent (what we're preventing)

First, show what happens WITHOUT Agentic OS. On any regular Linux box:

```bash
# Simulate what an AI agent does when you ask it to "get me logos in PNG format"
# The agent notices it needs imagemagick to convert formats, so it installs it.
dnf install -y imagemagick
# Works fine. No one asked. Agent now has full system access.
```

**This is the problem.** The agent acted autonomously and modified the system.

### Scenario B — The same agent on Agentic OS

```bash
# Register the agent with the minimal (default) profile
atomic-agent-ctl agent register logo-finder \
  --profile minimal \
  --exec /usr/bin/python3

# Start it in its sandbox
atomic-agent-ctl agent start logo-finder

# Open a second terminal and watch the audit log
# journalctl -o json wraps the audit payload inside MESSAGE as a JSON string,
# so pipe through fromjson before selecting fields.
journalctl -t atomic-audit -f -o json | jq '.MESSAGE | fromjson | {
  time: .timestamp,
  type,
  agent: .agent_id,
  action: .action_type,
  resource,
  decision
}'
```

Now simulate the agent trying to install imagemagick:

```bash
# This is what the agent would do — atomicagentd evaluates it first
atomic-agent-ctl policy eval logo-finder \
  --action exec \
  --resource /usr/bin/dnf

# Output:
# [DENY] agent=logo-finder action=exec resource=/usr/bin/dnf profile=minimal
#   Reasons:
#     - exec not permitted in minimal profile
```

Watch the audit log — the denial appears immediately as a JSON event:

```json
{
  "timestamp": "2026-03-04T12:00:00Z",
  "type": "policy.deny",
  "severity": "WARNING",
  "agent_id": "logo-finder",
  "action_type": "exec",
  "resource": "/usr/bin/dnf",
  "policy_profile": "minimal",
  "decision": "deny",
  "message": "agent logo-finder exec on /usr/bin/dnf: deny"
}
```

**The agent cannot install software. Full stop.**

### Scenario C — Credential theft (prompt injection)

Show what happens if a malicious prompt tricks the agent into reading SSH keys:

```bash
atomic-agent-ctl policy eval logo-finder \
  --action filesystem_read \
  --resource /root/.ssh/id_rsa

# [DENY] agent=logo-finder action=filesystem_read resource=/root/.ssh/id_rsa
```

And the cloud metadata SSRF attack (the most dangerous one):

```bash
atomic-agent-ctl policy eval logo-finder \
  --action network_connect \
  --resource 169.254.169.254

# [DENY] agent=logo-finder action=network_connect resource=169.254.169.254
# The agent cannot steal cloud IAM credentials.
```

### Scenario D — What the agent CAN do

```bash
# Writing to its workspace is allowed
atomic-agent-ctl policy eval logo-finder \
  --action filesystem_write \
  --resource /var/lib/atomic/agents/logo-finder/workspace/logo.png

# [ALLOW] agent=logo-finder action=filesystem_write ...

# Reading its own workspace is allowed
atomic-agent-ctl policy eval logo-finder \
  --action filesystem_read \
  --resource /var/lib/atomic/agents/logo-finder/workspace/results.json

# [ALLOW]
```

The agent is productive within its sandbox. It just cannot escape it.

---

## Part 3: Profile Upgrade — Controlled Trust Escalation (5 minutes)

Show how an operator can grant more trust to a specific agent in a controlled way.

```bash
# Register a developer agent (can use internet + dev tools)
atomic-agent-ctl agent register code-agent \
  --profile developer \
  --exec /usr/bin/python3

# This agent CAN call public APIs
atomic-agent-ctl policy eval code-agent \
  --action network_connect \
  --resource 8.8.8.8

# [ALLOW] agent=code-agent action=network_connect resource=8.8.8.8 profile=developer

# But STILL cannot reach the metadata service
atomic-agent-ctl policy eval code-agent \
  --action network_connect \
  --resource 169.254.169.254

# [DENY] — metadata is blocked for ALL profiles, no exceptions
```

---

## Part 4: Falco Runtime Detection (5 minutes)

While policy engine runs at the application level, Falco watches at the **kernel** level.
Even if an agent somehow bypassed atomicagentd, Falco catches it.

```bash
# Watch Falco events
journalctl -u falco -f

# In another terminal, simulate an agent process trying to read /etc/shadow
# (In a real demo you'd trigger this from inside the agent sandbox)
sudo -u nobody cat /etc/shadow 2>/dev/null || true

# Falco fires:
# {"rule":"AI Agent Credential File Access","priority":"CRITICAL",
#  "output":"AI agent credential access attempt (agent_id=... file=/etc/shadow)"}

# atomicagentd receives the webhook and kills the agent:
# journalctl -t atomic-audit | grep "agent.killed"
```

---

## Part 5: OS Atomic Update (2 minutes)

Demonstrate that the OS itself updates like a container pull — and can be rolled back instantly.

```bash
# Check current OS image
bootc status
# Shows: image reference, version, digest

# Stage an upgrade (non-disruptive, runs in background)
sudo bootc upgrade
# Downloads the new image layers

# Apply on next reboot
sudo systemctl reboot

# After reboot — if something is wrong:
sudo bootc rollback
sudo systemctl reboot
# Back to previous version in 30 seconds.
```

This is the key differentiator from traditional OS updates: **rollback is as fast as a reboot**.

---

## Demo Flow for Summit (Suggested Order)

| Step | What to show | Key message |
|---|---|---|
| 1 | `bootc status` | The OS is an OCI image. Updates are container pulls. |
| 2 | `atomic-agent-ctl agent register` | Agents are registered, not just spawned |
| 3 | Policy eval on `/usr/bin/dnf` → DENY | Agents cannot install software |
| 4 | Policy eval on `/etc/shadow` → DENY | Agents cannot steal credentials |
| 5 | Policy eval on `169.254.169.254` → DENY | No SSRF / metadata theft |
| 6 | Policy eval on workspace write → ALLOW | Agents still get their job done |
| 7 | Audit log tail | Every action is logged, structured, queryable |
| 8 | `bootc upgrade && bootc rollback` | Zero-downtime OS updates with instant rollback |

**The single-sentence pitch**: *Agentic OS is the missing safety layer between your AI agents and your infrastructure — built into the OS, enforced at the kernel, with a policy you can read and audit.*

---

## Troubleshooting

**`atomicagentd` not enabled on boot**
```bash
# The service is enabled via a systemd preset baked into /usr (immutable).
# If you see "disabled" it means you're on an older image build.
# Fix on the running VM (writes to writable /etc):
systemctl enable atomicagentd.service
# The next image rebuild includes the preset fix automatically.
```

**`atomicagentd` won't start**
```bash
journalctl -u atomicagentd -n 50
# Check: policy files present at /usr/share/atomic/policy/
# Check: /run/atomic/ directory exists with correct permissions
# Check: atomic-daemon system user exists
id atomic-daemon
```

**`agent start` returns "Interactive authentication required"**
```bash
# atomicagentd needs polkit permission to start transient systemd units.
# The permission is granted via /usr/share/polkit-1/rules.d/10-atomic-daemon.rules
# (baked into /usr). On an older image, apply the workaround to writable /etc:
mkdir -p /etc/polkit-1/rules.d
cat > /etc/polkit-1/rules.d/10-atomic-daemon.rules << 'EOF'
polkit.addRule(function(action, subject) {
    if ((action.id === "org.freedesktop.systemd1.manage-units" ||
         action.id === "org.freedesktop.systemd1.manage-unit-files") &&
        subject.user === "atomic-daemon") {
        return polkit.Result.YES;
    }
});
EOF
systemctl restart atomicagentd
```

**Policy eval returns "agent not registered"**
```bash
# You must register the agent before evaluating policy
atomic-agent-ctl agent register <name> --profile minimal --exec /usr/bin/python3
```

**`bootc upgrade` fails**
```bash
# Verify the image reference
bootc status
# Check network connectivity to ghcr.io
curl -I https://ghcr.io/v2/
```

**Falco not starting**
```bash
systemctl status falco
# On some VMs the eBPF probe needs kernel headers:
dnf install -y kernel-devel-$(uname -r)
```
