---
title: Install the control plane
---

The pages in this section are alternatives, not sequential steps.
Choose the one that matches the platform your team already operates, or intends to operate, then follow it through to the end.
Before you start, check the [installation requirements](./requirements.md) for the platform you choose.

## Choose a platform

| Where the control plane runs | Guide                                               | Best suited to                                                               |
|------------------------------|-----------------------------------------------------|------------------------------------------------------------------------------|
| Kubernetes (recommended)     | [Kubernetes](./kubernetes/index.md)                 | Production deployments, including high availability with multiple replicas   |
| Kubernetes                   | [Rancher](./rancher.md)                             | Kubernetes clusters your team already manages through Rancher                |
| Kubernetes                   | [OpenShift](./openshift.md)                         | Red Hat OpenShift clusters, which need specific security context settings    |
| Container runtime            | [Docker](./docker.md)                               | A single machine, such as a proof of concept or a small team deployment      |
| Linux                        | [RPM for Red Hat Enterprise Linux (RHEL)](./rpm.md) | A single Red Hat Enterprise Linux host, or another RHEL-family distribution  |
| Cloud platform               | [Cloud providers](./cloud/index.md)                 | A virtual machine on AWS, Google Cloud, or Azure, or a marketplace listing   |
| Other                        | [Community install methods](./community.md)         | Platforms Coder doesn't package, where a community-contributed method exists |

Kubernetes is the recommended platform for deployments that others depend on, and the [Coder Validated Architecture](../plan/sizing/index.md) reference designs assume it.
The Kubernetes guide covers managed clusters such as Amazon Elastic Kubernetes Service (EKS), Azure Kubernetes Service (AKS), and Google Kubernetes Engine (GKE).
It also covers lightweight distributions such as K3s.
To install on a cluster you already run in the cloud, use the Kubernetes guide rather than [Cloud providers](./cloud/index.md).

## Deployments without internet access

If the deployment has no internet access, read [Air-gapped deployments](../prepare/airgap.md) before you install, and use it alongside the guide for your platform.
To try an air-gapped deployment from start to finish on one machine, follow [Install Coder on K3s in an air-gapped environment](./kubernetes/k3s-air-gapped.md).

If you need to size the deployment before you choose a platform, refer to [Plan your deployment](../plan/index.md).

## Before you move on

At the end of this phase you should be able to reach the Coder access URL, sign in as the first administrator, and see the deployment reported as healthy.
If you prepared a license and an identity provider application in [Prepare prerequisites](../prepare/index.md), apply them now so that the deployment you validate is the one you intend to run.
Then [create your first template](../../templates/creating-templates.md#from-a-starter-template) and build a workspace from it to confirm the deployment works end to end.

Next: [Validate your deployment](../validate/index.md).

<children></children>
