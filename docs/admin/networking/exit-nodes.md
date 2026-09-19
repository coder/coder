# Route workspace egress through exit nodes (Premium)

This guide is for Coder deployment administrators and template administrators who need to control and observe outbound workspace traffic.
It covers how to deploy exit nodes, bind them to templates, define egress policy, and establish the required infrastructure enforcement boundary.

> [!IMPORTANT]
> You need permission to create exit nodes in the organization and update the affected templates.
> Template administrators can bind templates to exit nodes they can read.

## Overview

An exit node is an organization-scoped tailnet peer that terminates workspace egress, applies a policy, and reports flows to coderd.
The workspace agent sends TCP, UDP, and DNS traffic to an exit node over the Coder tailnet.

The trust model has 3 parts:

1. The exit node identifies the workspace agent from its tailnet source address.
2. The exit node decides whether each flow is allowed and connects to the destination for allowed flows.
3. Coderd attributes reported flows to the workspace and writes them to the Connection Log.

The exit node rejects traffic from tailnet addresses that don't belong to agents currently bound to it.
Coderd also ignores a reported flow when the agent's template isn't bound to the reporting exit node.

## Enforcement boundary

> [!WARNING]
> In-container `iptables` rules capture traffic for the agent's local proxy, but they are not the security boundary.
> A process with sufficient control of the workspace network namespace can bypass or disrupt in-container capture.
> You must apply a Kubernetes NetworkPolicy, cloud security group, or equivalent control outside the workspace that denies all other egress.

The external policy must allow only the destinations required to reach:

- The workspace DNS resolver, so the agent can resolve exempt control-plane hosts.
- Coderd.
- DERP and STUN endpoints in use by the deployment.
- Each exit node's advertised WireGuard UDP endpoint.

Start with the [Kubernetes NetworkPolicy example](../../../examples/exit-node/kubernetes-networkpolicy.yaml) or the [AWS security group example](../../../examples/exit-node/aws-security-group.tf).
Replace every documentation address, selector, and port with values from your deployment before applying an example.

The agent receives control-plane exemptions in `protocol/host:port` form.
Coderd and DERP use their exact TCP ports, STUN uses its exact UDP port, and each advertised WireGuard endpoint uses its exact UDP port.
These narrowly scoped exemptions keep the tailnet and coderd connection available while transparent capture is active.
They do not replace the external deny policy.

The agent places exemption host names in `NO_PROXY` for applications that honor the explicit proxy environment variables.
`NO_PROXY` has no protocol or port semantics and does not control transparent capture.

## Quickstart

This Quickstart creates one exit node, starts it with a deny-by-default policy, binds a template, and verifies an egress flow.

### Create a policy

Copy the [sample exit node policy](../../../examples/exit-node/policy.yaml) to the host that runs the exit node.
The sample allows selected GitHub HTTPS traffic, DNS queries needed to resolve allowed names, and a UDP service.

Review the policy before continuing.
A deny-by-default policy can interrupt package managers, source control, IDE extensions, and other workspace tools.

### Create an exit node

Run the following command from an authenticated `coder` CLI session:

```sh
coder exit-node create primary-egress
```

The command prints the new exit node's token and tailnet address.
Save the token when it is printed because coderd stores only its hash and cannot display it again.

### Start the exit node

Run the exit node on a host that can reach coderd and every destination allowed by the policy:

```sh
export CODER_EXIT_NODE_TOKEN='<exit-node-id>:<secret>'

coder exit-node server \
  --primary-access-url https://coder.example.com \
  --policy ./policy.yaml \
  --wireguard-listen-port 51820 \
  --wireguard-endpoint 203.0.113.10:51820 \
  --prometheus-address 127.0.0.1:2112
```

The command prints `Starting exit node` with the exit node ID and tailnet address, then streams logs.
The fixed WireGuard listen port and advertised endpoint let the agent and external egress policy use a stable UDP destination.
Omit both WireGuard options if the exit node must connect only through DERP.
Use `--block-direct-connections` to force that behavior.

Keep `--listen-port` at its default value of `3128`.
Coderd currently sends port `3128` to workspace agents, so a different tailnet CONNECT port prevents agents from reaching the exit node.
The separate `--wireguard-listen-port` option selects the host's UDP port for direct tailnet connections.

Use `--upstream-dns <ip[:port]>` one or more times when the exit node must use specific resolvers.
Without this option, the exit node uses its own resolver configuration for destination names and DNS streams.

Send `SIGHUP` to reload the YAML file without restarting the server:

```sh
kill -HUP <exit-node-process-id>
```

A valid file replaces the policy atomically.
If the reload fails, the previous policy remains active and the exit node logs the error.

### Bind a template

Bind the exit node to a template and request transparent capture:

```sh
coder templates edit my-template \
  --exit-node primary-egress \
  --exit-node-enforce
```

The command prints `Updated template metadata` after a successful update.
Running workspace agents receive manifest changes from coderd.
Changes to the exit node preference list apply in place without closing the local proxy listeners.
Changes that require new enforcement rules take effect after a workspace restart, as described later in this section.

The `--exit-node-enforce` option requests in-container transparent capture on Linux.
If the agent can't manage netfilter with root or passwordless `sudo`, it logs that enforcement is unavailable and continues in advisory proxy mode.
The agent still sets uppercase and lowercase `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, and `NO_PROXY` variables for processes it starts.

The agent's local listeners use stable ports by default:

- TCP proxy: `41280`.
- DNS over TCP and UDP: `41253`.
- Redirected UDP: `41254`.

If a preferred port is unavailable, the agent uses an ephemeral port and logs a warning.
Stable ports let the agent update the ordered exit node list in place without closing its listeners or invalidating installed capture rules.
If an agent is already using ephemeral fallback ports, those listener ports also remain unchanged during in-place updates.

Changes to the enforcement setting or control-plane exemptions require different netfilter rules.
After capability lockdown, the agent logs a warning and defers these changes until the workspace restarts.

Apply the external enforcement policy from the [Enforcement boundary](#enforcement-boundary) section before treating the template policy as enforced.

### Verify an egress flow

Start a workspace from the template and connect to an allowed destination:

```sh
curl -I https://github.com
```

Open **Deployment** > **Connection Log** and filter the connection type to **Egress**.
The row identifies the workspace, destination, protocol, and whether the exit node allowed or denied the flow.
Refer to [Connection logs](../monitoring/connection-logs.md) for filtering and export options.

## Policy YAML reference

The policy contains an ordered `rules` list and an optional `default` action.
Rules are evaluated in order, and the first matching rule wins.
The default is `deny` when `default` is omitted.

Each rule has an optional unique `id` and exactly one of `allow` or `deny`.
A rule matches only when every criterion it specifies matches.
An omitted criterion matches any value.

```yaml
default: deny
rules:
  - id: allow-github-https
    allow:
      hosts:
        - github.com
        - "*.github.com"
      ports: [443]
      protocols: [tcp]

  - id: deny-private-networks
    deny:
      cidrs:
        - 10.0.0.0/8
        - 192.168.0.0/16
      protocols: [tcp, udp]

  - id: deny-quic
    deny:
      ports: [443]
      protocols: [udp]

  - id: allow-development-ports
    allow:
      ports: ["8000-8999"]
      protocols: [tcp]
```

### Rule criteria

- `hosts` accepts exact DNS names, a leading `*.suffix` glob, or `*` for any known host.
  A suffix glob matches subdomains only, so `*.example.com` doesn't match `example.com`.
- `cidrs` accepts an IP prefix or a single IPv4 or IPv6 address.
- `ports` accepts a port number or an inclusive `start-end` range from 1 through 65535.
- `protocols` accepts `tcp`, `udp`, and `dns`.

Host names are normalized to lowercase without a trailing dot.
An unknown host doesn't match a host criterion.

### DNS policy behavior

DNS rules differ from TCP and UDP rules because name resolution happens before a connection can use the result.
For a DNS query, the policy considers only the query name and protocol.
It ignores `cidrs` and `ports`.

A DNS rule is eligible when either condition applies:

- The rule includes `dns` in `protocols`.
- The rule has `hosts` and omits `protocols`.

The first eligible rule with a matching host decides the query.
If no DNS rule matches, the query is allowed regardless of the top-level `default` action.

Use a host rule without `protocols` to deny both resolution and later TCP or UDP connections:

```yaml
- id: deny-malware-domain
  deny:
    hosts:
      - malware.example
      - "*.malware.example"
```

Use `protocols: [dns]` when you want a rule to apply only to DNS queries:

```yaml
- id: deny-dns-query
  deny:
    hosts: [tracking.example]
    protocols: [dns]
```

This DNS-only rule applies to A and AAAA queries before the agent synthesizes an address.
The exit node returns DNS `REFUSED` for a policy denial.
Responses such as `NXDOMAIN` and `SERVFAIL` from the exit node's upstream resolver pass through to the workspace.

### IP-literal targets and host rules

Host-based allow rules are strict by default for TCP connections made directly to an IP address.
Before application data arrives, the exit node knows the IP, port, and protocol but might not know the host.
A host-only allow rule can't match at that stage, so another IP, CIDR, port, protocol, or default rule must allow the connection.

After an initial allow, the exit node can learn a host from TLS SNI or the HTTP `Host` header and evaluate the policy again.
A later denial closes the connection, which the application observes as a reset or end of file.

The `--provisional-host-allow` option lets host-based allow rules provisionally match before the host is known.
The exit node evaluates the policy again after it learns a host.
This option weakens host enforcement because traffic can begin before the host is verified.
Use it only when compatibility requires this behavior.

## DNS routing

When transparent capture is active, the agent redirects workspace DNS over UDP and TCP to a loopback DNS proxy.
For every non-exempt A or AAAA query, the agent first sends the query through the exit node's DNS stream.
The exit node applies DNS policy and queries its upstream resolver when policy allows the name.

A policy denial returns `REFUSED` to the workspace.
Upstream responses such as `NXDOMAIN` and `SERVFAIL` pass through unchanged.
If no exit node completes the DNS stream, the agent returns `SERVFAIL`.

After a successful upstream response, the agent synthesizes an A record from the placeholder range `198.18.0.0/15`.
The agent normalizes the upstream minimum TTL to between 5 and 300 seconds, then caps its decision cache at 30 seconds.
The synthesized record carries the remaining decision-cache lifetime.
When an application connects to the fake address, the agent translates it back to the name and sends the name to the exit node.

The agent returns an empty successful answer for an allowed non-exempt AAAA query.
This behavior directs dual-stack clients to the fake IPv4 path while the exit node can still resolve an IPv6-only destination by name.

A bounded least-recently-used cache stores up to 1024 DNS decisions and reduces exit node round trips.
The cache key includes both the normalized name and query type, so A and AAAA decisions are separate.
Each decision remains cached for between 5 and 30 seconds, based on the upstream response TTL.
A synthesized A answer carries the remaining cache lifetime, with a minimum TTL of 1 second.

The agent clears this cache and reopens the DNS relay stream when it applies a manifest update or switches exit nodes.
After an exit node policy reload, new DNS decisions reflect the policy within at most 30 seconds, plus any TTL still cached by the workspace application or resolver.

The fake-IP pool holds 65536 name mappings by default, and its pinned-entry cap defaults to the same value.
If the pool or pinned-entry limit is exhausted, the agent returns `SERVFAIL`.

Every workspace process's DNS request, over TCP or UDP, is redirected to the local DNS proxy.
There is no blanket TCP port 53 exemption for system resolvers.
For an exempt host name, the agent's DNS proxy queries the workspace's system resolver over TCP using an agent-only socket mark that bypasses capture.
Only agent-owned sockets receive this mark, so workspace processes can't use the direct resolver path.

Exempt names include `localhost`, `host.docker.internal`, and host names from protocol-specific control-plane exemptions.
Transparent control-plane bypass rules still match the exact protocol, resolved address, and port.
Other DNS record types travel through the exit node DNS stream without fake-address synthesis.

Applications that use their own resolver endpoint or DNS over HTTPS can avoid DNS name capture.
Their later TCP or UDP flows still reach the exit node when the external boundary and transparent capture cover the destination, but host-based policy might receive no host name.

## UDP behavior

On Linux, the agent first attempts transparent IPv4 UDP capture with TPROXY, policy routing, and an `IP_TRANSPARENT` socket.
This path preserves the original destination and supports return datagrams that appear to come from that destination.

If TPROXY or policy routing isn't available, the agent installs a `REDIRECT` fallback.
`REDIRECT` loses the original destination for non-DNS UDP, so the agent drops that traffic and logs a warning instead of sending it to the wrong destination.
DNS remains captured separately.

IPv6 UDP uses the `REDIRECT` fallback because the transparent UDP proxy listens on IPv4 loopback.
Treat non-DNS IPv6 UDP as unsupported for enforced routing.

The agent attempts to install `ip6tables` rules when the workspace namespace has non-loopback IPv6 enabled.
If that installation fails, the agent disables IPv6 for the namespace to keep enforcement fail closed.
If IPv6 can't be disabled, the agent removes the IPv4 rules and runs in advisory mode instead of leaving partial enforcement active.

The agent drops UDP datagrams larger than 1400 bytes.
Each client and destination pair has a separate exit node stream, which closes after 60 seconds without traffic.

## Capability lockdown

After the Linux agent installs the capture rules, it locks down `CAP_NET_ADMIN` and `CAP_NET_RAW` for processes that the agent starts.
Linux 5.17 and later permits either capability to set `SO_MARK`, which could reproduce the agent-only bypass mark.
The agent clears ambient capabilities, removes both capabilities from the inheritable and bounding sets on every operating-system thread, then verifies each thread through `/proc/self/task/*/status`.
Children can't regain either capability, even after changing user IDs.

All-thread lockdown requires a Coder agent binary built with `CGO_ENABLED=0`.
Production Coder binaries are static and meet this requirement.
A cgo-linked custom agent can't complete lockdown and falls back to advisory mode after enforcement cleanup.

The lockdown covers only processes spawned by the agent.
It doesn't remove capabilities from a separate container entrypoint process or from processes that existed before the agent applied lockdown.
Run the agent as the container entrypoint and don't run any other privileged process in the workspace container.
A separate process with `CAP_NET_ADMIN` or `CAP_NET_RAW` remains a residual bypass risk because it can modify networking or set the bypass socket mark.
At the pod or container level, drop `NET_RAW` when the workspace workload doesn't require raw sockets.

Capability lockdown has 2 operational consequences:

- The agent can't remove the rules by running another privileged command.
  The rules persist until the container's network namespace exits.
- Rule changes, including control-plane exemption or enforcement changes, can't be applied after lockdown.
  The agent keeps the current listeners and rules, logs a warning, and applies the new configuration after a workspace restart.

Restart the workspace container, not only the agent process, after changing enforced egress configuration.
An external NetworkPolicy or security group remains the required boundary throughout agent restarts.

## High availability

Bind a template to multiple exit nodes by repeating `--exit-node` in preference order:

```sh
coder templates edit my-template \
  --exit-node primary-egress \
  --exit-node secondary-egress \
  --exit-node-enforce
```

The agent starts with the first exit node.
CONNECT request and response negotiation has a 10-second deadline.
A dial failure, timeout, end of file, or malformed HTTP response marks the node as failed, and the agent tries later nodes in order.

The agent classifies a well-formed HTTP response by status code.
A `200` response succeeds.
A policy denial `403`, malformed request `400`, or upstream or resolution failure `502` is terminal and doesn't trigger failover.
Other server errors, including `500`, `503`, and `504`, mark the node unhealthy and trigger failover.
Other well-formed non-5xx responses are also terminal.
Failed nodes have a 5-second retry delay.
During subsequent new connection attempts, the agent probes higher-priority nodes after their retry delay and returns to a preferred node after it recovers.

Failover applies to new TCP, UDP, and DNS streams.
Existing streams remain attached to the node that accepted them until they close.
Run the same policy and compatible DNS resolver configuration on every exit node in one failover set.

## Observability

Set `--prometheus-address` on each exit node to expose its metrics endpoint.
The exit node registers Go and process collectors plus these feature metrics:

| Metric                                                        | Description                                                                |
|---------------------------------------------------------------|----------------------------------------------------------------------------|
| `coder_exit_node_flows_total{decision="allow|deny"}`          | Flows handled by policy decision.                                          |
| `coder_exit_node_bytes_total{direction="in|out"}`             | Proxied bytes from the destination or agent.                               |
| `coder_exit_node_active_flows`                                | Flows currently being proxied.                                             |
| `coder_exit_node_unknown_source_total`                        | Connections rejected because the tailnet source isn't a known bound agent. |
| `coder_exit_node_policy_reload_total{result="success|error"}` | Policy reload attempts by result.                                          |
| `coder_exit_node_flow_reports_sent_total`                     | Flow reports delivered to coderd.                                          |
| `coder_exit_node_flow_reports_dropped_total`                  | Reports dropped because the bounded outgoing queue was full.               |

The Connection Log records egress rows with the workspace, agent, destination, destination IP, protocol, decision, matching rule ID, reason, and connection times.
For completed flows, the reason includes inbound and outbound byte counts.
Denied flows have status code `403`.
Allowed flows have status code `0` in the stored connection record.

Flow reporting is asynchronous and retries delivery failures.
If the report queue overflows, the exit node drops the oldest reports and tells coderd how many were lost with the next successful batch.
Coderd inserts an `exit-node-report-gap` Connection Log marker with a reason such as `dropped 25 exit node flow reports`.
Alert on both `coder_exit_node_flow_reports_dropped_total` and gap markers because the missing flows can't be reconstructed.

## Limitations

- Transparent enforcement is available only on Linux with `iptables` and either root or passwordless `sudo`.
  Other platforms and insufficiently privileged agents use advisory proxy environment variables.
- Capability lockdown removes `CAP_NET_ADMIN` and `CAP_NET_RAW`, requires a `CGO_ENABLED=0` agent, and applies only to processes spawned by the agent.
  Run the agent as the only privileged container entrypoint, and drop `NET_RAW` at the container level when the workload doesn't need it.
- In-container netfilter capture isn't a security boundary.
  Enforcement requires an external deny policy.
- If `ip6tables` installation fails while IPv6 is enabled, the agent disables IPv6 in the namespace.
  If that also fails, the agent removes IPv4 enforcement and uses advisory mode.
- Non-DNS IPv6 UDP isn't transparently relayed when IPv6 enforcement is active.
  IPv6 UDP uses the destination-losing `REDIRECT` fallback.
- UDP datagrams larger than 1400 bytes are dropped.
- Fake-IP mappings and pinned mappings are bounded at 65536 entries by default.
  Exhaustion causes DNS `SERVFAIL` responses.
- Changes to the exit node preference list apply in place and preserve the local listeners.
  Enforcement-setting and control-plane exemption changes require a workspace restart after capability lockdown.
- Server-first TCP protocols that connect to IP literals send no client bytes for host sniffing.
  Host-based policy receives no host unless verified DNS information was available before the connection.
- TCP protocols without TLS SNI or an HTTP `Host` header can leave the host unknown for IP-literal targets.
- UDP connections to IP literals have no host sniffing.
  Host-based UDP rules can't match unless the destination came from the agent's fake-IP DNS mapping.
- Clients that use custom resolvers, hard-coded addresses, or DNS over HTTPS can prevent the agent from associating a DNS name with a later flow.
- Transparent denials can appear to applications as a connection reset, end of file, or timeout instead of an HTTP policy error.
- Exit node flow reports use a bounded queue.
  A long reporting outage can create permanent gaps that coderd marks in the Connection Log.

## Learn more

- [Connection logs](../monitoring/connection-logs.md)
- [Coder networking](./index.md)
- [High availability](./high-availability.md)
