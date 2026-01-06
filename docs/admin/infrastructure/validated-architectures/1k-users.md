# Reference Architecture: up to 1,000 users

The 1,000 users architecture is designed to cover a wide range of workflows.
Examples of subjects that might utilize this architecture include medium-sized
tech startups, educational units, or small to mid-sized enterprises.

The recommendations on this page apply to deployments with up to the following limits. If your needs
exceed any of these limits, consider increasing deployment resources or moving to the [next-higher
architectural tier](./2k-users.md).

| Users | Concurrent Running Workspaces | Concurrent Builds |
|-------|-------------------------------|-------------------|
| 1000  | 600                           | 60                |

## Hardware recommendations

### Coderd

| vCPU | Memory | Replicas |
|------|--------|----------|
| 2    | 8 GB   | 3        |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `2000m`
  - Set Memory request and limit to `8Gi`
- Coderd does not typically benefit from high performance disks like SSDs (unless you are co-locating provisioners).
- For small deployments (ca. 100 users, 10 concurrent workspace builds), it is
  acceptable to deploy provisioners on `coderd` replicas.
- Coderd instances should be deployed in the same region as the database.

### Provisioners

| vCPU | Memory | Replicas |
|------|--------|----------|
| 1    | 1 GB   | 60       |

**Notes**:

- "General purpose" virtual machines, such as N4-series in GCP or M8-series in AWS work well.
- If deploying on Kubernetes:
  - Set CPU request and limit to `1000m`
  - Set Memory request and limit to `1Gi`
- If deploying on virtual machines, stack up to 30 provisioners per machine with a cummensurate amount of memory and CPU.
- Provisioners benefit from high performance disks like SSDs.
- For small deployments (ca. 100 users, 10 concurrent workspace builds), it is
  acceptable to deploy provisioners on `coderd` nodes.
- If deploying workspaces to multiple clouds or multiple Kubernetes clusters, divide the provisioner replicas among the
  clouds or clusters according to expected usage.

### Database

| vCPU | Memory | Replicas |
|------|--------|----------|
| 8    | 30 GB  | 1        |

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
