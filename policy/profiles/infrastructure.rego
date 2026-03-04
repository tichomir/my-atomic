# Atomic OS - Infrastructure Agent Profile
# For trusted infrastructure automation agents.
# Requires explicit operator authorization to assign.
#
# Capabilities:
#   - Read/write own workspace + read of /etc (no writes to /etc)
#   - Full network access (public + private, EXCEPT metadata services)
#   - Exec with full tooling allowlist
#   - External tool calls
#   - Workspace size limit: 50GB
#   - Memory limit: 2GB
#   - CPU quota: 200%
#   - ALL actions require audit logging

package atomic.agent.profiles.infrastructure

import future.keywords.in
import future.keywords.if

profile_name := "infrastructure"

# Network: full access except metadata endpoints
network_allowed := true
private_network_allowed := true

# Infrastructure agents may NEVER reach cloud metadata services
# (enforced at the kernel level via iptables rules set by atomicagentd)
metadata_endpoints_blocked := true

# Exec: full tooling
exec_allowed := true

additional_binaries := {
	"/usr/bin/npm",
	"/usr/bin/pip3",
	"/usr/bin/go",
	"/usr/bin/make",
	"/usr/bin/docker",
	"/usr/bin/podman",
	"/usr/bin/kubectl",
	"/usr/bin/helm",
	"/usr/bin/terraform",
	"/usr/bin/ansible",
	"/usr/bin/ansible-playbook",
	"/usr/bin/aws",
	"/usr/bin/gcloud",
	"/usr/bin/az",
	"/usr/bin/jq",
	"/usr/bin/yq",
	"/usr/bin/sed",
	"/usr/bin/awk",
	"/usr/bin/grep",
	"/usr/bin/find",
	"/usr/bin/tar",
	"/usr/bin/rsync",
	"/usr/bin/ssh",
	"/usr/bin/scp",
	"/usr/sbin/ip",
	"/usr/sbin/iptables",
	"/usr/bin/systemctl",
}

# Workspace quota - 50GB
workspace_quota_bytes := 53687091200

# Tool calls - all categories
allowed_tool_types := {
	"read_file",
	"write_file",
	"list_directory",
	"execute_command",
	"http_request",
	"search_web",
	"read_url",
	"cloud_api",
	"database_query",
	"secret_read",  # Read from secrets manager (not write)
}

external_tools_allowed := true

# Infrastructure agents may read /etc for system inspection
# but NEVER write to it
etc_read_allowed := true
etc_write_allowed := false

# All infrastructure actions must be audited (this is enforced in base policy)
force_audit_all := true
