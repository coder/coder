# Route workspace egress through exit nodes (Premium)

This guide is for Coder deployment and template administrators who need to control and observe outbound workspace traffic.
It covers exit node deployment, template bindings, egress policy, high availability, and the required external enforcement boundary.

> [!IMPORTANT]
> You need permission to create exit nodes in the organization and update the affected templates.
> Template administrators can bind templates to exit nodes they can read.

## How exit nodes work

An exit node is an organization-scoped logical resource with one authentication token and one or more replicas.
Each replica terminates workspace egress, applies policy, and reports flows to Coder.
Replicas share the logical exit node's token and policy, while templates remain bound to the logical resource.

Workspace agents send TCP, UDP, and DNS traffic to live replicas over the Coder tailnet.
Coder assigns each replica its tailnet identity and authorizes tunnels only from agents bound to that logical exit node.
Replicas reject unknown agents, and Coder ignores reports from unbound agents.

A complete deployment has 3 enforcement layers:

1. The agent captures workspace traffic and sends it over the tailnet.
2. The exit node identifies the source agent and allows or denies each flow.
3. An external network control blocks any egress path that bypasses the agent.

Coder attributes exit node flow reports to workspaces in the Connection Log.

## Establish the external enforcement boundary

> [!WARNING]
> In-container `iptables` rules are not a security boundary.
> A process that controls the workspace network namespace can bypass or disrupt them.
> Apply a Kubernetes NetworkPolicy, cloud security group, or equivalent external control, or workspace traffic can bypass the exit node.

Deny all workspace egress except the exact destinations and ports required for:

- The workspace DNS resolver, which resolves exempt control-plane hosts.
- Coder.
- The deployment's DERP and STUN endpoints.
- Each replica's advertised WireGuard UDP endpoint.

Start with the [Kubernetes NetworkPolicy example](../../../examples/exit-node/kubernetes-networkpolicy.yaml) or [AWS security group example](../../../examples/exit-node/aws-security-group.tf).
Replace every example address, selector, and port before applying it.

The agent receives control-plane exemptions in `protocol/host:port` form.
Coder and DERP use exact TCP ports, while STUN and advertised WireGuard endpoints use exact UDP ports.
These netfilter exemptions also permit workspace processes to reach the same endpoints.
Only agent-created resolver and transparent reply sockets use the agent-only bypass mark.

The agent also adds exempt host names to `NO_PROXY` for applications that honor proxy environment variables.
`NO_PROXY` has no protocol or port semantics, does not control transparent capture, and does not replace the external deny policy.

## Deploy and configure an exit node

The following procedure creates a logical exit node, starts a replica with a deny-by-default policy, binds a template, and verifies a flow.

### Create the policy

Copy the [sample exit node policy](../../../examples/exit-node/policy.yaml) to the replica host.
The sample allows selected GitHub HTTPS traffic, the required DNS queries, and one UDP service.

Review the policy before continuing.
A deny-by-default policy can interrupt package managers, source control, IDE extensions, and other workspace tools.

### Create the logical exit node

Run the following command from an authenticated `coder` CLI session:

```sh
coder exit-node create primary-egress
```

The command prints the logical exit node's token once.
Save it because Coder stores only its hash and cannot display it again.
Every replica of this logical exit node uses the same token.

### Start a replica

Run each replica on a host that can reach Coder and every policy-allowed destination:

```sh
export CODER_EXIT_NODE_TOKEN='<exit-node-id>:<secret>'

coder exit-node server \
  --primary-access-url https://coder.example.com \
  --policy ./policy.yaml \
  --wireguard-listen-port 51820 \
  --wireguard-endpoint 203.0.113.10:51820 \
  --prometheus-address 127.0.0.1:2112
```

The command prints `Starting exit node replica` with a generated replica ID and tailnet address, then streams logs.
Each process start generates a fresh replica ID by default.

Use the server options as follows:

| Option                                               | Operational effect                                                                                                                       |
|------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| `--replica-id <uuid>`                                | Overrides the generated ID. Omit it for normal and autoscaled deployments because a deregistered ID cannot register again.               |
| `--listen-port`                                      | Sets the tailnet CONNECT port. Keep the default `3128`; agents currently connect to that port.                                           |
| `--wireguard-listen-port` and `--wireguard-endpoint` | Provide a stable direct UDP destination for agents and external egress policy.                                                           |
| `--block-direct-connections`                         | Forces DERP-only connections. Use it when you omit both WireGuard options.                                                               |
| `--upstream-dns <ip[:port]>`                         | Selects a resolver. Repeat the option for multiple resolvers. Without it, the replica uses its host resolver configuration.              |
| `--provisional-host-allow`                           | Lets host-based allow rules match before the host is known. This weakens host enforcement because traffic can start before verification. |
| `--prometheus-address`                               | Exposes the replica metrics endpoint.                                                                                                    |

The WireGuard UDP port is separate from the tailnet CONNECT port.

Send `SIGHUP` to reload the YAML policy without restarting the replica:

```sh
kill -HUP <exit-node-process-id>
```

A valid file replaces the policy atomically.
If validation fails, the previous policy remains active and the replica logs the error.

Use the same raw YAML file on every replica of a logical exit node.
Replicas report its hash, and Coder flags a policy mismatch when live siblings report different hashes.
A mismatched replica also logs a warning and sets `coder_exit_node_policy_mismatch` to `1`.

### Prepare the workspace agent

Complete a rolling upgrade of affected workspace agents to Agent API 2.14 or later before binding their templates.
Restart any older running workspace so it receives replica-aware exit node configuration.

Transparent enforcement is available only for a Linux agent that:

- Has `iptables` available.
- Runs as the workspace container's root entrypoint.
- Holds `CAP_NET_ADMIN` in the agent process, typically through the container's `NET_ADMIN` capability.
- Uses a `CGO_ENABLED=0` agent binary, as official production binaries do.

Passwordless `sudo` is not sufficient because it does not grant the agent process the capabilities required for lockdown.
If enforcement setup or lockdown fails, the agent removes installed rules, logs that enforcement is unavailable, and continues in advisory proxy mode.
Other platforms also use advisory mode.

In both modes, the agent sets uppercase and lowercase `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, and `NO_PROXY` variables for processes it starts.

### Bind the template

Bind the logical exit node and request transparent capture:

```sh
coder templates edit my-template \
  --exit-node primary-egress \
  --exit-node-enforce
```

The command prints `Updated template metadata` after a successful update.
Apply the [external enforcement boundary](#establish-the-external-enforcement-boundary) before treating the policy as enforced.

The agent uses these preferred local listener ports:

| Listener             |    Port |
|----------------------|--------:|
| TCP proxy            | `41280` |
| DNS over TCP and UDP | `41253` |
| Redirected UDP       | `41254` |

If a preferred port is unavailable, the agent selects an ephemeral port and logs a warning.
Listener ports remain stable while live replica sets or logical node preferences change, including when the agent already uses fallback ports.

Changes to enforcement or control-plane exemptions require new netfilter rules.
After capability lockdown, the agent defers those changes until the workspace container restarts.
Restarting only the agent process is insufficient because the rules persist for the life of the network namespace.

Removing all exit node bindings also requires a workspace container restart.
Until then, the enforced agent keeps its proxy active with no replicas, so egress fails closed.

### Verify a flow

Start a workspace from the template, then connect to an allowed destination:

```sh
curl -I https://github.com
```

Open **Deployment** > **Connection Log** and filter the connection type to **Egress**.
The row identifies the workspace, destination, protocol, and policy decision.
Refer to [Connection logs](../monitoring/connection-logs.md) for filtering and export options.

## Define policy rules

A policy contains an ordered `rules` list and an optional `default` action.
The first matching rule wins, and the default is `deny` when omitted.

Each rule has an optional unique `id` and exactly one `allow` or `deny` action.
A rule matches when every specified criterion matches, and an omitted criterion matches any value.

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

| Criterion   | Accepted values and behavior                                                                                                     |
|-------------|----------------------------------------------------------------------------------------------------------------------------------|
| `hosts`     | Exact DNS names, a leading `*.suffix` glob, or `*` for any known host. A suffix glob matches subdomains but not the bare suffix. |
| `cidrs`     | An IP prefix or one IPv4 or IPv6 address.                                                                                        |
| `ports`     | A port or inclusive `start-end` range from `1` through `65535`.                                                                  |
| `protocols` | `tcp`, `udp`, or `dns`.                                                                                                          |

Host names are normalized to lowercase without a trailing dot.
An unknown host does not match a host criterion.

### Control DNS policy

For DNS queries, policy considers only the normalized query name and protocol, and ignores `cidrs` and `ports`.
A DNS rule is eligible if it includes `dns` in `protocols`, or if it has `hosts` and omits `protocols`.
The first eligible matching rule decides the query.
If no DNS rule matches, the query is allowed regardless of the top-level default.

A host rule without `protocols` applies to DNS and later TCP or UDP connections:

```yaml
- id: deny-malware-domain
  deny:
    hosts:
      - malware.example
      - "*.malware.example"
```

Use `protocols: [dns]` for DNS-only policy:

```yaml
- id: deny-dns-query
  deny:
    hosts: [tracking.example]
    protocols: [dns]
```

DNS denials return `REFUSED`.
Upstream responses such as `NXDOMAIN` and `SERVFAIL` pass through unchanged.

### Control IP-literal targets

For a direct connection to an IP address, the replica initially knows only the IP, port, and protocol.
A host-only allow rule cannot match unless verified DNS information is already available, so another criterion or the default must allow the initial connection.

The replica can later learn a host from TLS SNI or an HTTP `Host` header and evaluate policy again.
A later denial closes the connection, which applications can observe as a reset or end of file.

The `--provisional-host-allow` option permits the initial connection based on a host rule, then evaluates again after learning the host.
Use it only when compatibility requires traffic to begin before host verification.

Server-first TCP protocols, protocols without TLS SNI or an HTTP `Host` header, and UDP to IP literals might never provide a host.
Host-based UDP rules can match when the destination came from the agent's fake-IP DNS mapping.

## Understand DNS routing

With transparent capture active, the agent redirects workspace DNS over TCP and UDP to its loopback proxy.
For each non-exempt A or AAAA query, the agent sends the query through an exit node DNS stream.
The replica applies policy and uses its upstream resolver for allowed names.

The following outcomes apply:

| Condition                                           | Workspace result                                                             |
|-----------------------------------------------------|------------------------------------------------------------------------------|
| Policy denies the query                             | `REFUSED`                                                                    |
| Upstream resolver returns `NXDOMAIN` or `SERVFAIL`  | The response passes through unchanged.                                       |
| No replica completes the DNS stream                 | `SERVFAIL`                                                                   |
| An allowed non-exempt AAAA query completes          | An empty successful answer directs dual-stack clients to the fake IPv4 path. |
| The fake-IP pool or pinned-entry limit is exhausted | `SERVFAIL`                                                                   |

After a successful A or AAAA response, the agent synthesizes an A record from `198.18.0.0/15`.
When an application connects to that address, the agent translates it back to the name, and the replica can resolve an IPv6-only destination by name.

The agent normalizes the upstream minimum TTL to between 5 and 300&nbsp;seconds and caches the decision for at most 30&nbsp;seconds.
The synthesized record carries the remaining cache lifetime, with a minimum TTL of 1&nbsp;second.
The cache stores up to `1024` least-recently-used decisions, keyed separately by normalized name and query type.

The agent clears the cache and reopens the DNS stream when it applies a manifest update or switches replicas.
After a policy reload, new decisions reflect the policy within 30&nbsp;seconds, plus any TTL cached by the workspace application or resolver.

The fake-IP pool and pinned-entry cap each default to `65536` mappings.

All workspace DNS requests are redirected, with no general TCP port `53` exemption.
For exempt names, the agent queries the workspace resolver over TCP with its agent-only bypass mark.
Exempt names include `localhost`, `host.docker.internal`, and hosts from protocol-specific control-plane exemptions.
The resulting bypass rules still match the exact protocol, address, and port.
Other DNS record types use the exit node stream without fake-address synthesis.

Custom resolver endpoints, hard-coded addresses, and DNS over HTTPS can prevent the agent from associating a host with a later flow.
The external boundary and transparent capture still route covered TCP and UDP traffic, but host policy might receive no name.

## Understand UDP and IPv6 behavior

On Linux, the agent attempts transparent IPv4 UDP capture with TPROXY, policy routing, and an `IP_TRANSPARENT` socket.
This path preserves the original destination and source identity of return datagrams.

If TPROXY or policy routing is unavailable, the agent uses `REDIRECT`.
Because `REDIRECT` loses the original destination for non-DNS UDP, the agent drops that traffic and logs a warning.
DNS remains captured separately.

IPv6 UDP also uses `REDIRECT`, so non-DNS IPv6 UDP is unsupported for enforced routing.
If `ip6tables` setup fails while non-loopback IPv6 is enabled, the agent turns off IPv6 in the namespace to fail closed.
If it cannot turn off IPv6, it removes IPv4 rules and uses advisory mode instead of leaving partial enforcement active.

The agent drops UDP datagrams larger than `1400` bytes.
Each client and destination pair uses a separate stream that closes after 60&nbsp;seconds without traffic.

## Lock down workspace capabilities

After installing capture rules, the agent removes `CAP_NET_ADMIN` and `CAP_NET_RAW` from ambient, inheritable, and bounding capability sets for every agent thread.
Linux 5.17 and later permits either capability to set `SO_MARK`, so removing both prevents agent-started processes from reproducing the bypass mark.
The agent verifies the lockdown before it treats enforcement as active.

A cgo-linked custom agent cannot complete all-thread lockdown and falls back to advisory mode after cleanup.

Lockdown applies only to processes the agent starts.
Run the agent as the container entrypoint, do not run another privileged process in the workspace container, and drop `NET_RAW` at the container level when the workload does not need raw sockets.
Processes that existed before lockdown or retain either capability remain a bypass risk.

Capability lockdown makes enforced configuration immutable for the life of the workspace network namespace.
Replica set and logical node preference changes apply immediately, but enforcement and exemption changes require a workspace container restart.
Keep the external NetworkPolicy or security group active during all agent and workspace restarts.

## Configure high availability and failover

Run multiple replicas with the shared token and identical policy, either independently or with the [Kubernetes Deployment example](../../../examples/exit-node/kubernetes-deployment.yaml).
Do not set `--replica-id` in an autoscaled deployment.

Replica lifecycle uses these timings and statuses:

| Item                         | Behavior                                                              |
|------------------------------|-----------------------------------------------------------------------|
| Heartbeat                    | Every 5&nbsp;seconds.                                                 |
| Live threshold               | Last heartbeat is less than 15&nbsp;seconds old.                      |
| Stale cleanup                | Coder checks every 5&nbsp;seconds and publishes the updated live set. |
| Graceful shutdown            | Deregisters immediately and permanently stops that replica ID.        |
| Replica retry delay          | 5&nbsp;seconds after a failed connection.                             |
| CONNECT negotiation deadline | 10&nbsp;seconds for request and response.                             |

Agents load-distribute across replicas within a logical exit node.
Logical nodes retain the preference order configured on the template.
Bind multiple logical nodes by repeating `--exit-node`:

```sh
coder templates edit my-template \
  --exit-node primary-egress \
  --exit-node secondary-egress \
  --exit-node-enforce
```

A dial failure, timeout, end of file, or malformed HTTP response marks the replica failed and triggers failover.
For a well-formed response, the status code controls the result:

| Status                                | Result                                                                                                                   |
|---------------------------------------|--------------------------------------------------------------------------------------------------------------------------|
| `200`                                 | Success.                                                                                                                 |
| `400`, `403`, or `502`                | Terminal result with no failover. These represent a malformed request, policy denial, or upstream or resolution failure. |
| `500`, `503`, `504`, or another `5xx` | Replica failure and failover.                                                                                            |
| Other non-`5xx`                       | Terminal result with no failover.                                                                                        |

After the retry delay, new connections probe higher-priority replicas and logical nodes again.
Failover applies only to new TCP, UDP, and DNS streams.
Existing streams remain on their accepting replica until they close.

If all bound logical nodes have no live replicas, egress fails closed:

- Explicit proxy requests receive `503`.
- Transparent TCP connections close.
- UDP is dropped.
- DNS returns `SERVFAIL`.

The agent recovers when Coder publishes a live replica.

When a new replica advertises a WireGuard endpoint after an enforced workspace installs its exemptions, the agent reaches it through DERP.
Restart the workspace to add the endpoint to its external and netfilter exemptions.
Run compatible DNS resolver configuration and identical policy across every replica and logical node in one failover set.

## Monitor exit nodes

Set `--prometheus-address` on each replica to expose Go, process, and exit node metrics:

| Metric                                                        | Description                                            |
|---------------------------------------------------------------|--------------------------------------------------------|
| `coder_exit_node_flows_total{decision="allow|deny"}`          | Flows by policy decision.                              |
| `coder_exit_node_bytes_total{direction="in|out"}`             | Proxied bytes from the destination or agent.           |
| `coder_exit_node_active_flows`                                | Flows currently being proxied.                         |
| `coder_exit_node_unknown_source_total`                        | Connections rejected from an unknown or unbound agent. |
| `coder_exit_node_policy_reload_total{result="success|error"}` | Policy reload attempts by result.                      |
| `coder_exit_node_policy_mismatch`                             | Whether a live sibling reports another policy hash.    |
| `coder_exit_node_flow_reports_sent_total`                     | Flow reports delivered to Coder.                       |
| `coder_exit_node_flow_reports_dropped_total`                  | Reports dropped because the outgoing queue was full.   |

Use `coder exit-node list` to inspect logical nodes:

| Status         | Meaning                            |
|----------------|------------------------------------|
| `healthy`      | At least one replica is live.      |
| `unreachable`  | Replicas exist, but none are live. |
| `unregistered` | No replica has registered yet.     |

The command also shows the live replica count and policy mismatch state.

Use `coder exit-node replicas <name|id>` to inspect recorded replicas, including version, tailnet address, policy hash, and last update:

| Status    | Meaning                                              |
|-----------|------------------------------------------------------|
| `live`    | The last heartbeat is less than 15&nbsp;seconds old. |
| `stale`   | No heartbeat arrived for at least 15&nbsp;seconds.   |
| `stopped` | The replica deregistered explicitly.                 |

Connection Log egress rows contain the workspace, agent, destination, destination IP, protocol, decision, matching rule ID, reason, and connection times.
Completed flow reasons include inbound and outbound byte counts.
Denied flows store status code `403`, while allowed flows store `0`.

Flow reporting is asynchronous and retries delivery failures.
If the bounded queue fills, the replica drops the oldest reports and includes the loss count in its next successful batch.
Coder inserts an `exit-node-report-gap` marker with a reason such as `dropped 25 exit node flow reports`.
Alert on `coder_exit_node_flow_reports_dropped_total` and gap markers because lost flows cannot be reconstructed.

Transparent denials can appear to applications as a reset, end of file, or timeout instead of an HTTP policy error.

## Learn more

- [Connection logs](../monitoring/connection-logs.md)
- [Coder networking](./index.md)
- [High availability](./high-availability.md)
