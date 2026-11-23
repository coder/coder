---
title: "Reference architecture: up to 600 concurrent workspaces"
---

This architecture supports up to 600 concurrent running workspaces.
It covers a wide range of workflows, for example at medium-sized tech startups, educational units, or small to mid-sized enterprises.

The recommendations on this page apply to deployments with up to the following limits.
If your needs exceed any of these limits, consider increasing deployment resources or moving to the [next-higher architectural tier](./1200-workspaces.md).
To learn what each limit measures, refer to [Node sizing](./index.md#node-sizing).

| Concurrent running workspaces | Concurrent workspace builds |
|------------------------------:|----------------------------:|
|                           600 |                          60 |

## Hardware recommendations

These recommendations assume a deployment across three regions: `coderd` in the primary region, a workspace proxy in each of the other two regions, and workspaces and provisioners split evenly across all three.
The provisioner replica count is the total across all regions.

### Coderd

| vCPU | Memory    | Replicas |
|------|-----------|----------|
| 2    | 8&nbsp;GB | 3        |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `2000m`
  - Set Memory request and limit to `8Gi`
- `coderd` does not typically benefit from high performance disks like solid-state drives (SSDs), unless you are co-locating provisioners.
- For small deployments with about 10 concurrent workspace builds, it is acceptable to deploy provisioners on `coderd` replicas.
- `coderd` instances should be deployed in the same region as the database.
- Set `CODER_PG_CONN_MAX_OPEN` to `60` and `CODER_PG_CONN_MAX_IDLE` to `30`.
  To learn how to tune these values, refer to [Connection pool tuning](../../validate/scale-coder.md#connection-pool-tuning).

### Provisioners

| vCPU | Memory    | Replicas |
|------|-----------|----------|
| 1    | 1&nbsp;GB | 60       |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `1000m`
  - Set Memory request and limit to `1Gi`
- If deploying on virtual machines, stack up to 30 provisioners per machine with a commensurate amount of memory and CPU.
- Provisioners benefit from high performance disks like SSDs.
- For small deployments with about 10 concurrent workspace builds, it is acceptable to deploy provisioners on `coderd` nodes.
- If deploying workspaces to multiple clouds or multiple Kubernetes clusters, divide the provisioner replicas among the clouds or clusters according to expected usage.

### Database

| vCPU | Memory     | Replicas |
|------|------------|----------|
| 8    | 30&nbsp;GB | 1        |

**Notes**:

- "General purpose" virtual machines, such as the M8-series in AWS work well.
- Deploy in the same region as `coderd`.
- Set `max_connections` to at least 500.

### Workspaces

The following resource requirements are for the Coder Workspace Agent, which runs alongside your end users' work, and as such should be interpreted as the _bare minimum_ requirements for a Coder workspace.
Size your workspaces to fit the use case your users will be undertaking.
If in doubt, choose sizes based on the development environments your users are migrating from onto Coder.

| vCPU | Memory      |
|------|-------------|
| 0.1  | 128&nbsp;MB |

## Footnotes for AWS instance types

- For production deployments, we recommend using non-burstable instance types, such as `m5` or `c5`, instead of burstable instances, such as `t3`.
  Burstable instances can experience significant performance degradation once CPU credits are exhausted, leading to poor user experience under sustained load.
