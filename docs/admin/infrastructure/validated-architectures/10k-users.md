# Reference Architecture: up to 10,000 users

The 10,000 users architecture targets enterprises with an extremely large global workforce of technical professionals or
applications requiring lots of simultaneous workspaces (for example, Agentic AI).

The recommendations on this page apply to deployments with up to the following limits. If your needs
exceed any of these limits, consider increasing deployment resources.

| Users | Concurrent Running Workspaces | Concurrent Builds |
|-------|-------------------------------|-------------------|
| 10000 | 6000                          | 600               |

**Observability**: Deploy monitoring solutions to gather Prometheus metrics and
visualize them with Grafana to gain detailed insights into infrastructure and
application behavior. This allows operators to respond quickly to incidents and
continuously improve the reliability and performance of the platform.

## Hardware recommendations

### Coderd

| vCPU | Memory | Replicas |
|------|--------|----------|
| 4    | 12 GB  | 10       |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `4000m`
  - Set Memory request and limit to `12Gi`
- Coderd does not typically benefit from high performance disks like SSDs (unless you are co-locating provisioners).
- Coderd instances should be deployed in the same region as the database.

### Workspace Proxies

If you choose to deploy workspaces in multiple geographic regions, provision
[Workspace Proxies](../../networking/workspace-proxies.md) in each region.

| vCPU | Memory | Replicas |
|------|--------|----------|
| 4    | 12 GB  | 10       |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `4000m`
  - Set Memory request and limit to `12Gi`
- Workspace Proxies do not typically benefit from high performance disks like SSDs.

### Provisioners

| vCPU | Memory | Replicas |
|------|--------|----------|
| 1    | 1 GB   | 180      |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `1000m`
  - Set Memory request and limit to `1Gi`
- If deploying on virtual machines, stack up to 30 provisioners per machine with a commensurate amount of memory and CPU.
- Provisioners benefit from high performance disks like SSDs.
- [Do not run provisioners on Coderd nodes](../../provisioners/index.md#disable-built-in-provisioners) at this scale.
- If deploying workspaces to multiple clouds or multiple Kubernetes clusters, divide the provisioner replicas among the
  clouds or clusters according to expected usage.

### Database

| vCPU | Memory | Replicas |
|------|--------|----------|
| 64   | 240 GB | 1        |

**Notes**:

- "General purpose" virtual machines, such as the M8-series in AWS work well.
- Deploy in the same region as `coderd`

### Workspaces

The following resource requirements are for the Coder Workspace Agent, which runs alongside your end users work, and as
such should be interpreted as the _bare minimum_ requirements for a Coder workspace. Size your workspaces to fit the use
case your users will be undertaking. If in doubt, chose sizes based on the development environments your users are
migrating from onto Coder.

| vCPU | Memory |
|------|--------|
| 0.1  | 128 MB |

## Footnotes for AWS instance types

- For production deployments, we recommend using non-burstable instance types,
  such as `m5` or `c5`, instead of burstable instances, such as `t3`.
  Burstable instances can experience significant performance degradation once
  CPU credits are exhausted, leading to poor user experience under sustained load.
