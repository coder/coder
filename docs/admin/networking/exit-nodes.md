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

The agent receives coderd, DERP, STUN, and advertised WireGuard endpoints as control-plane exemptions.
These exemptions keep the tailnet and coderd connection available while transparent capture is active.
They do not replace the external deny policy.

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
New and reconnecting workspace agents receive the exit node configuration from coderd.

The `--exit-node-enforce` option requests in-container transparent capture on Linux.
If the agent can't manage netfilter with root or passwordless `sudo`, it logs that enforcement is unavailable and continues in advisory proxy mode.
The agent still sets uppercase and lowercase `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, and `NO_PROXY` variables for processes it starts.

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

The exit node returns DNS `REFUSED` for a policy denial and `SERVFAIL` when an allowed upstream exchange fails.

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
For a non-exempt name, an A query receives a short-lived placeholder address from `198.18.0.0/15`.
When an application connects to that address, the agent translates it back to the name and sends the name to the exit node.

The agent returns an empty successful answer for non-exempt AAAA queries.
This behavior directs dual-stack clients to the placeholder IPv4 path while the exit node can still resolve an IPv6-only destination by name.

Queries for exempt host names use the workspace's system resolvers over TCP.
Exempt names include `localhost`, `host.docker.internal`, and named coderd, DERP, or STUN endpoints.
Control-plane IP addresses and advertised exit node WireGuard endpoints bypass capture by destination address.
Other DNS record types travel through a long-lived DNS stream to the exit node.
The exit node applies DNS policy per query and forwards allowed queries to its configured upstream resolvers.

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

The agent drops UDP datagrams larger than 1400 bytes.
Each client and destination pair has a separate exit node stream, which closes after 60 seconds without traffic.

## Capability lockdown

After the Linux agent installs the capture rules, it drops `CAP_NET_ADMIN` from the process capability bounding set.
Child processes can't regain that capability, even after changing user IDs.
This protects the installed rules from workspace processes that share the agent's user identity.

Capability lockdown has 2 operational consequences:

- The agent can't remove the rules by running another privileged command.
  The rules persist until the container's network namespace exits.
- If the agent process replaces itself or restarts inside the same container, it can't reinstall or update the rules.
  The local proxy can return in advisory mode while the existing rules still reference the previous proxy ports.

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
When a new connection can't reach that node, the agent tries later nodes in order and uses the first successful connection.
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
- In-container netfilter capture isn't a security boundary.
  Enforcement requires an external deny policy.
- Non-DNS IPv6 UDP isn't transparently relayed.
  IPv6 uses the destination-losing `REDIRECT` fallback.
- UDP datagrams larger than 1400 bytes are dropped.
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
