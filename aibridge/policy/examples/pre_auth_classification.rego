# Pre-auth classification policy: annotate requests with their source IP and
# whether the client is connecting from a local (loopback or RFC-1918) address.
#
# source_ip is resolved in priority order:
#   1. X-Forwarded-For  — leftmost address in the comma-separated list
#   2. X-Real-IP        — single address set by some reverse proxies
#   3. x-remote-addr    — TCP remote address injected by the gateway
#
# Rule queried by the classify kind: data.gateway.annotations

local_ranges := [
	"127.0.0.0/8",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"::1/128",
	"fc00::/7",
]

# X-Forwarded-For may be "a, b, c" — take the leftmost (original client).
# else chains provide priority: XFF → X-Real-IP → TCP remote address.
source_ip := trim_space(split(input.headers["x-forwarded-for"], ",")[0]) if {
	input.headers["x-forwarded-for"] != ""
} else := input.headers["x-real-ip"] if {
	input.headers["x-real-ip"] != ""
} else := object.get(input.headers, "x-remote-addr", "")

default is_local := false

is_local if {
	source_ip != ""
	some cidr in local_ranges
	net.cidr_contains(cidr, source_ip)
}

annotations := {
	"source_ip": source_ip,
	"is_local": is_local,
}
