# Atomic OS - Filesystem Policy
# Defines path-based access control for agent filesystem operations.
#
# The workspace model:
#   - Each agent has a unique workspace at /var/lib/atomic/agents/<id>/workspace
#   - Agents may read/write ONLY within their workspace
#   - Read-only access to a limited set of system paths is granted
#   - Writes outside the workspace are always denied

package atomic.agent.filesystem

import future.keywords.in
import future.keywords.if

# Paths that are always readable by any agent (informational only)
always_readable := {
	"/etc/hostname",
	"/etc/os-release",
	"/etc/resolv.conf",
	"/proc/cpuinfo",
	"/proc/meminfo",
	"/proc/loadavg",
	"/proc/uptime",
	"/proc/version",
	"/usr/share/atomic/policy",
}

# Paths that are never readable (highest sensitivity)
never_readable := {
	"/etc/shadow",
	"/etc/gshadow",
	"/etc/master.passwd",
}

# Paths that are never writable (even by infrastructure profile)
never_writable_prefixes := [
	"/etc",
	"/usr",
	"/boot",
	"/run/atomic",
	"/proc",
	"/sys",
	"/dev",
]

is_never_writable if {
	some prefix in never_writable_prefixes
	startswith(input.action.resource, prefix)
}

is_never_readable if {
	input.action.resource in never_readable
}

# Validates that a write path is within the agent's workspace
is_workspace_write if {
	input.action.action_type in {"filesystem_write", "filesystem_delete"}
	startswith(input.action.resource, input.agent.workspace_path)
	not is_never_writable
}

# Validates that a read path is permitted
is_permitted_read if {
	input.action.action_type == "filesystem_read"
	startswith(input.action.resource, input.agent.workspace_path)
}

is_permitted_read if {
	input.action.action_type == "filesystem_read"
	input.action.resource in always_readable
}

is_permitted_read if {
	input.action.action_type == "filesystem_read"
	startswith(input.action.resource, "/usr/share/atomic/policy")
}
