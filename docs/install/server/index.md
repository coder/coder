# Install the control plane

The pages in this section are alternatives, not sequential steps.
Choose the one that matches the platform your team already operates, or intends to operate, then follow it through to the end.

## Choose a platform

| Platform                                    | Best suited to                                                               |
|---------------------------------------------|------------------------------------------------------------------------------|
| [Kubernetes](./kubernetes/index.md)         | Production deployments, including high availability with multiple replicas   |
| [Rancher](./rancher.md)                     | Kubernetes clusters your team already manages through Rancher                |
| [OpenShift](./openshift.md)                 | Red Hat OpenShift clusters, which need specific security context settings    |
| [Docker](./docker.md)                       | A single machine, such as a proof of concept or a small team deployment      |
| [Cloud providers](./cloud/index.md)         | A virtual machine on AWS, Google Cloud, or Azure, or a marketplace listing   |
| [Community install methods](./community.md) | Platforms Coder doesn't package, where a community-contributed method exists |

If your platform choice is still open, size the deployment first in [Plan your deployment](../plan/index.md).
The [Coder Validated Architecture](../plan/sizing/index.md) reference designs assume Kubernetes, which is the recommended platform for deployments that others depend on.

## Before you move on

At the end of this phase you should be able to reach the Coder access URL, sign in as the first administrator, and see the deployment reported as healthy.
If you prepared a license and an identity provider application in [Prepare prerequisites](../prepare/index.md), apply them now so that the deployment you validate is the one you intend to run.

Next: [Validate your deployment](../validate/index.md).

<children></children>
