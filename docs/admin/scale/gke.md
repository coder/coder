# Scaling Coder on Google Kubernetes Engine (GKE)

This is a reference architecture for Coder on [Google Kubernetes Engine](#). We regurily load test these environments with a standard [kubernetes example](https://github.com/coder/coder/tree/main/examples/templates/kubernetes) template.

> Performance and ideal node sizing depends on many factors, including workspace image and the [workspace sizes](https://github.com/coder/coder/issues/3519) you wish to give developers. Use Coder's [scale testing utility](./index.md#scale-testing-utility) to test your own deployment.

## 50 users

### Cluster configuration

- **Autoscaling profile**: `optimize-utilization`

- **Node pools**
  - Default
    - **Operating system**: `Ubuntu with containerd`
    - **Instance type**: `e2-highcpu-8`
    - **Min nodes**: `1`
    - **Max nodes**: `4`

### Coder settings

- **Replica count**: `1`
- **Provisioner daemons**: `30`
- **Template**: [kubernetes example](https://github.com/coder/coder/tree/main/examples/templates/kubernetes)

## 100 users

For deployments with 100+ users, we recommend running the Coder server in a separate node pool via taints, tolerations, and nodeselectors.

### Cluster configuration

- **Node pools**
  - Coder server
    - **Instance type**: `e2-highcpu-4`
    - **Operating system**: `Ubuntu with containerd`
    - **Autoscaling profile**: `optimize-utilization`
    - **Min nodes**: `2`
    - **Max nodes**: `4`
  - Workspaces
    - **Instance type**: `e2-highcpu-16`
    - **Node**: `Ubuntu with containerd`
    - **Autoscaling profile**: `optimize-utilization`
    - **Min nodes**: `3`
    - **Max nodes**: `10`

### Coder settings

- **Replica count**: `4`
- **Provisioner daemons**: `25`
- **Template**: [kubernetes example](https://github.com/coder/coder/tree/main/examples/templates/kubernetes)
