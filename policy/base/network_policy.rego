# Atomic OS - Network Policy
# Controls which network destinations agents may connect to.
#
# By default, agents have NO network access.
# The "developer" and "infrastructure" profiles extend this.

package atomic.agent.network

import future.keywords.in
import future.keywords.if

# Default: block all network connections
default allow_network := false

# Localhost is always allowed (agents may need to connect to local services)
allow_network if {
	input.action.action_type == "network_connect"
	net.cidr_contains("127.0.0.0/8", input.action.resource)
}

allow_network if {
	input.action.action_type == "network_connect"
	net.cidr_contains("::1/128", input.action.resource)
}

# Block connections to cloud metadata services (SSRF protection)
# These endpoints can expose instance credentials
blocked_endpoints := {
	"169.254.169.254",   # AWS/GCP/Azure metadata
	"fd00:ec2::254",     # AWS IPv6 metadata
	"metadata.google.internal",
	"169.254.170.2",     # AWS ECS credentials
}

is_blocked_endpoint if {
	input.action.resource in blocked_endpoints
}

is_blocked_endpoint if {
	# Block any attempt to reach the metadata CIDR
	net.cidr_contains("169.254.169.254/32", input.action.resource)
}

# Block RFC-1918 private ranges for minimal/developer profiles
# (infrastructure profile overrides this)
is_private_network if {
	net.cidr_contains("10.0.0.0/8", input.action.resource)
}

is_private_network if {
	net.cidr_contains("172.16.0.0/12", input.action.resource)
}

is_private_network if {
	net.cidr_contains("192.168.0.0/16", input.action.resource)
}
