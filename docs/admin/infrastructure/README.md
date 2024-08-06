# Infrastructure

Guides for setting up and scaling Coder infrastructure.

## Architecture

Coder is a self-hosted platform that runs on your own infrastructure. For large deployments, we recommend running the control plane on Kubernetes. Workspaces can be run as VMs or Kubernetes pods. The control plane (`coderd`) runs in a single region. However, workspace proxies, provisioners, and workspaces can run across regions or even cloud providers for the optimal developer experience.

Learn more about Coder's architecture and concepts: [Concepts](./architecture.md)
