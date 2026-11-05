---
title: "Reference architecture: up to 10,000 users"
---

The 10,000 users architecture targets enterprises with an extremely large global workforce of technical professionals or applications requiring lots of simultaneous workspaces (for example, Agentic AI).

The recommendations on this page apply to deployments with up to the following limits.
If your needs exceed any of these limits, consider increasing deployment resources.

| Users | Concurrent Running Workspaces | Concurrent Builds |
|-------|-------------------------------|-------------------|
| 10000 | 6000                          | 600               |

<include>../../../_includes/sizing/observability.md</include>

## Hardware recommendations

### Coderd

| vCPU | Memory     | Replicas |
|------|------------|----------|
| 4    | 12&nbsp;GB | 10       |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `4000m`
  - Set Memory request and limit to `12Gi`
- `coderd` does not typically benefit from high performance disks like solid-state drives (SSDs), unless you are co-locating provisioners.
- `coderd` instances should be deployed in the same region as the database.

### Workspace Proxies

If you choose to deploy workspaces in multiple geographic regions, provision [Workspace Proxies](../../../admin/networking/workspace-proxies.md) in each region.

| vCPU | Memory     | Replicas |
|------|------------|----------|
| 4    | 12&nbsp;GB | 10       |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `4000m`
  - Set Memory request and limit to `12Gi`
- Workspace Proxies do not typically benefit from high performance disks like SSDs.

### Provisioners

| vCPU | Memory    | Replicas |
|------|-----------|----------|
| 1    | 1&nbsp;GB | 600      |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `1000m`
  - Set Memory request and limit to `1Gi`
- If deploying on virtual machines, stack up to 30 provisioners per machine with a commensurate amount of memory and CPU.
- Provisioners benefit from high performance disks like SSDs.
- [Do not run provisioners on `coderd` nodes](../../operate/provisioners/index.md#disable-built-in-provisioners) at this scale.
- If deploying workspaces to multiple clouds or multiple Kubernetes clusters, divide the provisioner replicas among the clouds or clusters according to expected usage.

### Database

| vCPU | Memory      | Replicas |
|------|-------------|----------|
| 64   | 240&nbsp;GB | 1        |

**Notes**:

<include>../../../_includes/sizing/database-notes.md</include>

### Workspaces

<include>../../../_includes/sizing/workspace-agent-requirements.md</include>

## Footnotes for AWS instance types

<include>../../../_includes/sizing/aws-instance-footnotes.md</include>
