---
title: High Availability
---

High Availability (HA) mode solves for horizontal scalability and automatic
failover within a single region. When in HA mode, Coder continues using a single
Postgres endpoint.
[GCP](https://cloud.google.com/sql/docs/postgres/high-availability),
[AWS](https://docs.aws.amazon.com/prescriptive-guidance/latest/saas-multitenant-managed-postgresql/availability.html),
and other cloud vendors offer fully-managed HA Postgres services that pair
nicely with Coder.

For Coder to operate correctly, `coderd` instances should have low-latency
connections to each other so that they can effectively relay traffic between
users and workspaces no matter which `coderd` instance users or workspaces
connect to. We make a best-effort attempt to warn the user when inter-`coderd`
latency is too high, but if requests start dropping, this is one metric to
investigate.

We also recommend that you deploy all `coderd` instances such that they have
low-latency connections to Postgres. `coderd` often makes several database
round-trips while processing a single API request, so prioritizing low-latency
between `coderd` and Postgres is more important than low-latency between users
and `coderd`.

Note that this latency requirement applies _only_ to Coder services. Coder will
operate correctly even with few seconds of latency on workspace <-> Coder and
user <-> Coder connections.

## Setup

Coder automatically enters HA mode when multiple instances simultaneously
connect to the same Postgres endpoint.

> [!NOTE]
> When upgrading HA deployments, database migrations may require special
> handling to avoid lock contention. See
> [Upgrading Best Practices](../../install/operate/upgrade/best-practices.md) for
> recommended procedures.

HA brings one configuration variable to set in each `coderd` node:
`CODER_DERP_SERVER_RELAY_URL`. The HA nodes use these URLs to communicate with
each other. Inter-node communication is only required while using the embedded
relay (default). If you're using [custom relays](./index.md#custom-relays),
Coder ignores `CODER_DERP_SERVER_RELAY_URL` since Postgres is the sole
rendezvous for the Coder nodes.

`CODER_DERP_SERVER_RELAY_URL` will never be `CODER_ACCESS_URL` because
`CODER_ACCESS_URL` is a load balancer to all Coder nodes.

Coder also clusters its inter-node pubsub across HA nodes and needs each
replica's routable address to do so. This address comes from
`CODER_CLUSTER_HOST`, falling back to the host in `CODER_DERP_SERVER_RELAY_URL`
when it is unset. Set `CODER_CLUSTER_HOST` to each node's routable IP address.
Setting it is required when you use [custom relays](./index.md#custom-relays),
because `CODER_DERP_SERVER_RELAY_URL` is ignored and no fallback address is
available. If a node has neither value set, Coder logs an error and falls back
to PostgreSQL for pubsub.

Here's an example 3-node network configuration setup:

| Name      | `CODER_HTTP_ADDRESS` | `CODER_DERP_SERVER_RELAY_URL` | `CODER_CLUSTER_HOST` | `CODER_ACCESS_URL`       |
|-----------|----------------------|-------------------------------|----------------------|--------------------------|
| `coder-1` | `*:80`               | `http://10.0.0.1:80`          | `10.0.0.1`           | `https://coder.big.corp` |
| `coder-2` | `*:80`               | `http://10.0.0.2:80`          | `10.0.0.2`           | `https://coder.big.corp` |
| `coder-3` | `*:80`               | `http://10.0.0.3:80`          | `10.0.0.3`           | `https://coder.big.corp` |

## Kubernetes

If you installed Coder via
[our Helm Chart](../../install/server/kubernetes/index.md#4-install-coder-with-helm), just
increase `coder.replicaCount` in `values.yaml`.

If you installed Coder into Kubernetes by some other means, insert the relay URL
via the environment like so:

```yaml
env:
  - name: POD_IP
    valueFrom:
      fieldRef:
        fieldPath: status.podIP
  - name: CODER_DERP_SERVER_RELAY_URL
    value: http://$(POD_IP)
  - name: CODER_CLUSTER_HOST
    value: $(POD_IP)
```

Then, increase the number of pods.

## Up next

- [Read more on Coder's networking stack](./index.md)
- [Install on Kubernetes](../../install/server/kubernetes/index.md)
