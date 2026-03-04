# Atomic OS - Developer Agent Profile
# For development and CI/CD agents. More permissive than minimal.
#
# Capabilities:
#   - Read/write own workspace
#   - Filtered internet access (public internet only, no metadata services)
#   - Exec with expanded binary allowlist
#   - External tool calls allowed
#   - Workspace size limit: 10GB
#   - Memory limit: 1GB
#   - CPU quota: 150%

package atomic.agent.profiles.developer

import future.keywords.in
import future.keywords.if

profile_name := "developer"

# Network: public internet allowed, private/metadata ranges blocked
network_allowed := true

# Exec: allowed for development tools
exec_allowed := true

# Additional binaries beyond the base allowlist
additional_binaries := {
	"/usr/bin/npm",
	"/usr/bin/yarn",
	"/usr/bin/pip",
	"/usr/bin/pip3",
	"/usr/bin/go",
	"/usr/bin/cargo",
	"/usr/bin/make",
	"/usr/bin/gcc",
	"/usr/bin/clang",
	"/usr/bin/docker",
	"/usr/bin/podman",
	"/usr/bin/kubectl",
	"/usr/bin/helm",
	"/usr/bin/terraform",
	"/usr/bin/ansible",
	"/usr/bin/jq",
	"/usr/bin/yq",
	"/usr/bin/sed",
	"/usr/bin/awk",
	"/usr/bin/grep",
	"/usr/bin/find",
	"/usr/bin/tar",
	"/usr/bin/unzip",
	"/usr/bin/ssh",
	"/usr/bin/scp",
	"/usr/bin/rsync",
}

# Workspace quota - 10GB
workspace_quota_bytes := 10737418240

# Tool calls
allowed_tool_types := {
	"read_file",
	"write_file",
	"list_directory",
	"execute_command",
	"http_request",
	"search_web",
	"read_url",
}

external_tools_allowed := true

# Developer agents may NOT install packages to system paths
# (they can only write to their workspace)
system_package_install_allowed := false
