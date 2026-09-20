# Route workspace egress through exit nodes (Premium)

Workspaces hold source code, credentials, and AI agents that run arbitrary commands.
Most teams want to say exactly which external services a workspace can reach, and to know afterwards which workspace talked to what.
A cluster firewall can block by IP, but it cannot express "GitHub, npm, and the internal artifact registry" for one template and "nothing but PyPI" for another, and it cannot tell you which workspace made a connection.

Exit nodes solve that.
An exit node is a small service you run that terminates outbound workspace traffic, applies a hostname-aware allow or deny policy, and reports every flow to Coder with the workspace that produced it.
Workspaces reach the exit node over the Coder tailnet they already use, so you get per-template egress policy and a per-workspace Connection Log without new network plumbing inside the workspace.

This guide is for deployment and template administrators.
It explains the model, walks through a deployment, and then covers policy, high availability, and operations.

## How it works

Three pieces work together:

1. **The workspace agent** captures the workspace's TCP, UDP, and DNS traffic with netfilter rules and forwards it to an exit node over the tailnet.
   It then drops its own network capabilities so workspace processes cannot undo the rules.
2. **The exit node** identifies the sending workspace, evaluates policy against the destination host, address, port, and protocol, then either proxies the flow or rejects it.
   It reports every decision to Coder.
3. **An external network control** such as a Kubernetes NetworkPolicy or cloud security group denies every workspace egress path except Coder, DERP or STUN, and the exit nodes.
   This is the real security boundary; the agent's rules are the mechanism that makes allowed traffic attributable, not the thing that stops a determined bypass.

An exit node is an organization-scoped resource with one token.
You can run any number of identical replicas under that token; Coder tracks them by heartbeat and hands workspaces the live set.
Templates bind to one or more exit nodes in preference order.

> [!IMPORTANT]
> Creating exit nodes requires exit node permissions in the organization.
> Template administrators can bind templates to exit nodes they can read.

## Deploy an exit node

### Prerequisites

- A host for each replica that can reach Coder and every destination the policy allows.
- Workspace agents on Agent API 2.14 or later.
  Restart older running workspaces before binding their template; they cannot consume replica-aware configuration.
- For transparent enforcement, a Linux workspace image where the agent runs as the container's root entrypoint with `CAP_NET_ADMIN`, has `iptables`, and uses a `CGO_ENABLED=0` agent binary (the official binaries do).
  Passwordless `sudo` is not enough because the agent process itself must hold and then drop the capability.
  Any other setup, including other platforms, runs in advisory mode: the proxy is available through `HTTP_PROXY` style variables but traffic is not captured.

### 1. Create the exit node and save its token

```sh
coder exit-node create primary-egress
```

The token is printed once; Coder stores only a hash.
Every replica uses this same token.

### 2. Write a policy

Start from the [sample policy](../../../examples/exit-node/policy.yaml).
The default is deny, so review it against what your tooling needs: package managers, source control, IDE extensions, and any internal services.
Policy syntax is described in [Define policy](#define-policy).

### 3. Start one or more replicas

```sh
export CODER_EXIT_NODE_TOKEN='<exit-node-id>:<secret>'

coder exit-node server \
  --primary-access-url https://coder.example.com \
  --policy ./policy.yaml \
  --prometheus-address 127.0.0.1:2112
```

Each process registers as a replica with a fresh ID and streams logs.
Run several for availability; the [Kubernetes Deployment example](../../../examples/exit-node/kubernetes-deployment.yaml) runs three behind one token Secret and policy ConfigMap.

Useful options:

| Option                                               | Effect                                                                                                                                                                             |
|------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--wireguard-listen-port` and `--wireguard-endpoint` | Advertise a stable public UDP endpoint so enforced workspaces can exempt it and reach the replica directly instead of through DERP. See [Direct connections](#direct-connections). |
| `--block-direct-connections`                         | Force DERP only. Use it when you do not advertise an endpoint.                                                                                                                     |
| `--upstream-dns <ip[:port]>`                         | Pin the resolver the replica uses. Repeatable. Defaults to the host resolver.                                                                                                      |
| `--provisional-host-allow`                           | Let host rules admit a connection to an IP literal before the host is verified. Weakens host enforcement; see [IP literals](#ip-literals-and-late-hostnames).                      |
| `--replica-id <uuid>`                                | Fix the replica ID. Omit it in autoscaled deployments: a replica that deregisters can never register again under the same ID.                                                      |

Reload the policy without a restart by sending `SIGHUP`.
An invalid file is rejected and the previous policy stays active.

### 4. Apply the external boundary

> [!WARNING]
> Until this step is done, workspace traffic can bypass the exit node.

Deny all workspace egress except the exact destinations and ports for Coder, the DERP and STUN endpoints, the workspace's DNS resolver, and each replica's advertised WireGuard endpoint.
Adapt the [Kubernetes NetworkPolicy](../../../examples/exit-node/kubernetes-networkpolicy.yaml) or [AWS security group](../../../examples/exit-node/aws-security-group.tf) example, replacing every address, selector, and port.

The agent mirrors these exemptions as exact `protocol/host:port` netfilter rules, so workspace processes can reach the same endpoints directly.
That is intended: those endpoints are the control plane, not the internet.

### 5. Bind the template

```sh
coder templates edit my-template \
  --exit-node primary-egress \
  --exit-node-enforce
```

Repeat `--exit-node` to add lower-priority nodes for failover.
Omit `--exit-node-enforce` to offer the proxy through environment variables only, without capturing traffic.

### 6. Verify

From a workspace on the template:

```sh
curl -I https://github.com
```

Open **Deployment** > **Connection Log** and filter by connection type **Egress**.
Each row shows the workspace, destination, protocol, decision, matched rule, and byte counts.
See [Connection logs](../monitoring/connection-logs.md).

## Define policy

A policy is an ordered list of rules with a `default` action (`deny` when omitted).
The first rule whose every listed criterion matches decides the flow.

```yaml
default: deny
rules:
  - id: allow-github-https
    allow:
      hosts: [github.com, "*.github.com"]
      ports: [443]
      protocols: [tcp]

  - id: deny-private-networks
    deny:
      cidrs: [10.0.0.0/8, 192.168.0.0/16]
      protocols: [tcp, udp]

  - id: allow-development-ports
    allow:
      ports: ["8000-8999"]
      protocols: [tcp]
```

| Criterion   | Values                                                                                                                                         |
|-------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `hosts`     | Exact names, a `*.suffix` glob (matches subdomains, not the bare suffix), or `*` for any known host. Normalized to lowercase, no trailing dot. |
| `cidrs`     | An IP prefix or a single IPv4 or IPv6 address.                                                                                                 |
| `ports`     | A port or an inclusive `start-end` range.                                                                                                      |
| `protocols` | `tcp`, `udp`, or `dns`.                                                                                                                        |

### DNS rules

DNS queries are evaluated against the query name and protocol only.
A rule applies to DNS when it lists `dns` in `protocols`, or when it has `hosts` and no `protocols` at all, which is the common case of "block this domain everywhere".
Denied queries return `REFUSED`; if no DNS rule matches, the query is allowed regardless of `default`.
Upstream `NXDOMAIN` and `SERVFAIL` pass through unchanged.

### IP literals and late hostnames

When a workspace connects straight to an IP address, the exit node initially knows only the IP, port, and protocol, so a host-only allow rule cannot match.
Either allow the flow by `cidrs` or `ports`, or accept that it is denied.
If the exit node later sees a hostname in the TLS SNI or HTTP `Host` header, it evaluates policy again and closes the connection on a deny, which the application sees as a reset.
`--provisional-host-allow` reverses the order (admit on the host rule, verify after) for protocols that need it, at the cost of letting a few packets through before verification.

Names reach the exit node because the agent captures workspace DNS: every allowed answer is replaced by a synthetic address from `198.18.0.0/15` that the agent maps back to the name when the connection arrives.
Applications that bypass the resolver (hard-coded IPs, DNS over HTTPS) still have their traffic captured, but policy sees only an IP.

## High availability

Run several replicas per exit node with identical policy, and optionally bind several exit nodes in preference order.

- Replicas heartbeat every 5 seconds and are live while their last heartbeat is under 15 seconds old.
  Coder checks every 5 seconds and pushes the live set to running agents immediately, without a workspace restart.
- Agents spread load across the replicas of the preferred exit node and fail over to the next node when none are reachable.
  A failed replica is retried after 5 seconds; higher-priority nodes are re-probed on the same schedule, so traffic returns to the primary on its own.
- A graceful shutdown (`SIGTERM`) deregisters immediately.
  A killed replica is removed within about 15 seconds.
- If every bound exit node has no live replica, egress fails closed: the explicit proxy returns `503`, transparent TCP connections close, UDP is dropped, and DNS returns `SERVFAIL`.
  Traffic resumes as soon as a replica returns.
- Every replica reports a hash of its policy file.
  `coder exit-node list` shows **policy mismatch** and each replica sets `coder_exit_node_policy_mismatch` to `1` when live siblings disagree.

Only new connections fail over; established flows stay on their replica until they close.

### Direct connections

Replicas always connect to Coder's DERP relay, so they work with no inbound ports.
For throughput, give each replica a stable public UDP endpoint with `--wireguard-listen-port` and `--wireguard-endpoint`, and allow it in the external boundary.
Agents exempt that endpoint from capture and negotiate a direct WireGuard path, taking Coder out of the data path.
Because an enforced workspace cannot change its netfilter rules after startup, an endpoint that appears later is reached through DERP until the workspace restarts.

## Operations

### Monitoring

`coder exit-node list` shows each node as `healthy` (a replica is live), `unreachable` (replicas exist, none live), or `unregistered`, with the live replica count and mismatch state.
`coder exit-node replicas <name>` lists replicas as `live`, `stale`, or `stopped` with version, tailnet address, and policy hash.

With `--prometheus-address`, each replica exposes:

| Metric                                        | Meaning                                                 |
|-----------------------------------------------|---------------------------------------------------------|
| `coder_exit_node_flows_total{decision}`       | Flows by `allow` or `deny`.                             |
| `coder_exit_node_bytes_total{direction}`      | Proxied bytes, `in` from destination, `out` from agent. |
| `coder_exit_node_active_flows`                | Flows in progress.                                      |
| `coder_exit_node_unknown_source_total`        | Connections from agents not bound to this node.         |
| `coder_exit_node_policy_reload_total{result}` | `SIGHUP` reloads by `success` or `error`.               |
| `coder_exit_node_policy_mismatch`             | `1` when a live sibling has a different policy.         |
| `coder_exit_node_flow_reports_sent_total`     | Flow reports delivered to Coder.                        |
| `coder_exit_node_flow_reports_dropped_total`  | Reports lost because the outgoing queue was full.       |

Flow reporting is asynchronous.
If reports are dropped, Coder records an `exit-node-report-gap` row for the affected workspace; alert on the dropped counter because lost flows cannot be reconstructed.

### Changing configuration on running workspaces

After the agent installs its rules it locks itself down, so in an enforced workspace the netfilter rules are immutable for the life of the container:

- Adding, removing, or reordering replicas and exit nodes applies immediately.
- Turning enforcement on or off, changing control-plane exemptions, or adding a WireGuard endpoint takes effect when the workspace container restarts.
  Restarting only the agent process is not enough.
- Removing all exit nodes from a template leaves running enforced workspaces failing closed until they restart.

Keep the external boundary in place across restarts.

### Workspace image requirements

Lockdown removes `CAP_NET_ADMIN` and `CAP_NET_RAW` from the agent and everything it starts, and only from those.
Run the agent as the container entrypoint, do not run another privileged process in the workspace, and drop `NET_RAW` at the container level if the workload does not need it.
Processes that start before the agent or outside it keep whatever capabilities the container grants.

## Reference

Behavior an operator may need to know but rarely tune:

| Area              | Behavior                                                                                                                                                                                                                                                          |
|-------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Agent listeners   | TCP proxy `41280`, DNS `41253`, UDP `41254` on loopback. Falls back to ephemeral ports with a warning.                                                                                                                                                            |
| Proxy environment | The agent sets `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, and `NO_PROXY` (upper and lower case) for processes it starts. `NO_PROXY` lists exempt hosts only and does not affect transparent capture.                                                               |
| DNS cache         | Policy decisions are cached up to 30 seconds (floor 5 seconds), 1024 entries, keyed by name and record type. The synthetic A record's TTL is the remaining cache lifetime. AAAA answers are empty so clients use the IPv4 path.                                   |
| Fake IP pool      | 65536 mappings from `198.18.0.0/15`, least recently used first, pinned while a flow is active. Exhaustion returns `SERVFAIL`.                                                                                                                                     |
| UDP               | Linux IPv4 UDP is captured with TPROXY. If TPROXY is unavailable the agent falls back to `REDIRECT`, which cannot preserve the destination, so non-DNS UDP is dropped with a warning. Datagrams over 1400 bytes are dropped. Idle streams close after 60 seconds. |
| IPv6              | Non-DNS IPv6 UDP is not routed. If `ip6tables` setup fails the agent disables IPv6 in the namespace; if that also fails it removes its IPv4 rules and runs in advisory mode rather than enforce partially.                                                        |
| CONNECT results   | `200` success. `400`, `403`, `502` are final (bad request, policy deny, upstream failure). Other `5xx`, timeouts, and malformed responses trigger failover. Negotiation deadline 10 seconds.                                                                      |
| Connection Log    | Egress rows carry workspace, agent, destination, protocol, decision, rule ID, reason, and bytes. Denied flows store code `403`, allowed `0`. Transparent denials appear to applications as a reset or timeout.                                                    |

## Learn more

- [Connection logs](../monitoring/connection-logs.md)
- [Coder networking](./index.md)
- [High availability](./high-availability.md)
