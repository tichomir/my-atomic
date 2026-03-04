# Atomic OS - Minimal Agent Profile
# The most restrictive profile. Used for untrusted, exploratory, or sandboxed agents.
#
# Capabilities:
#   - Read/write own workspace only
#   - No network access
#   - No exec (cannot spawn processes)
#   - No tool_call to external services
#   - Workspace size limit: 1GB
#   - Memory limit: 256MB
#   - CPU quota: 50%

package atomic.agent.profiles.minimal

import future.keywords.if

# Profile identifier
profile_name := "minimal"

# Network: completely disabled
network_allowed := false

# Exec: disabled - agents cannot spawn subprocesses
exec_allowed := false

# Additional binary allowlist (extends base, but exec_allowed=false makes this moot)
additional_binaries := set()

# Workspace quota (bytes) - 1GB
workspace_quota_bytes := 1073741824

# Tool calls allowed for minimal profile
allowed_tool_types := {
	"read_file",
	"write_file",
	"list_directory",
}

# Minimal agents may not call external tools
external_tools_allowed := false
