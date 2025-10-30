# Reference Architecture: up to 10,000 users

> [!CAUTION]
> This page is a work in progress.
>
> We are actively testing different load profiles for this user target and will be updating
> recommendations. Use these recommendations as a starting point, but monitor your cluster resource
> utilization and adjust.

The 10,000 users architecture targets large-scale enterprises with development
teams in multiple geographic regions.

**Geographic Distribution**: For these tests we deploy on 3 cloud-managed Kubernetes clusters in
the following regions:

1. USA - Primary - Coderd collocated with the PostgreSQL database deployment.
2. Europe - Workspace Proxies
3. Asia - Workspace Proxies

**High Availability**: Typically, such scale requires a fully-managed HA
PostgreSQL service, and all Coder observability features enabled for operational
purposes.

**Observability**: Deploy monitoring solutions to gather Prometheus metrics and
visualize them with Grafana to gain detailed insights into infrastructure and
application behavior. This allows operators to respond quickly to incidents and
continuously improve the reliability and performance of the platform.

## Testing Methodology

### Workspace Network Traffic

6000 concurrent workspaces (2000 per region), each sending 10 kB/s application traffic.

Test procedure:

1. Create workspaces. This happens simultaneously in each region with 200 provisioners (and thus 600 concurrent builds).
2. Wait 5 minutes to establish baselines for metrics.
3. Generate 10 kB/s traffic to each workspace (originating within the same region & cluster).

After, we examine the Coderd, Workspace Proxy, and Database metrics to look for issues.

### API Request Traffic

To be determined.

## Hardware recommendations

### Coderd

These are deployed in the Primary region only.

| vCPU Limit     | Memory Limit | Replicas | GCP Node Pool Machine Type |
|----------------|--------------|----------|----------------------------|
| 4 vCPU (4000m) | 12 GiB       | 10       | `c2d-standard-16`          |

### Provisioners

These are deployed in each of the 3 regions.

| vCPU Limit      | Memory Limit | Replicas | GCP Node Pool Machine Type |
|-----------------|--------------|----------|----------------------------|
| 0.1 vCPU (100m) | 1 GiB        | 200      | `c2d-standard-16`          |

**Footnotes**:

- Each provisioner handles a single concurrent build, so this configuration implies 200 concurrent
  workspace builds per region.
- Provisioners are run as a separate Kubernetes Deployment from Coderd, although they may
  share the same node pool.
- Separate provisioners into different namespaces in favor of zero-trust or
  multi-cloud deployments.

### Workspace Proxies

These are deployed in the non-Primary regions only.

| vCPU Limit     | Memory Limit | Replicas | GCP Node Pool Machine Type |
|----------------|--------------|----------|----------------------------|
| 4 vCPU (4000m) | 12 GiB       | 10       | `c2d-standard-16`          |

**Footnotes**:

- Our testing implies this is somewhat overspecced for the loads we have tried. We are in process of revising these numbers.

### Workspaces

These numbers are for each of the 3 regions. We recommend that you use a separate node pool for user Workspaces.

| Users       | Node capacity        | Replicas                      | GCP              | AWS          | Azure             |
|-------------|----------------------|-------------------------------|------------------|--------------|-------------------|
| Up to 3,000 | 8 vCPU, 32 GB memory | 256 nodes, 12 workspaces each | `t2d-standard-8` | `m5.2xlarge` | `Standard_D8s_v3` |

**Footnotes**:

- Assumed that a workspace user needs 2 GB memory to perform
- Maximum number of Kubernetes workspace pods per node: 256
- As workspace nodes can be distributed between regions, on-premises networks
  and cloud areas, consider different namespaces in favor of zero-trust or
  multi-cloud deployments.

### Database nodes

We conducted our test using the `db-custom-16-61440` tier on Google Cloud SQL.

**Footnotes**:

- This database tier was only just able to keep up with 600 concurrent builds in our tests.
