# Coder Validated Architecture

Many customers operate Coder in complex organizational environments, consisting
of multiple business units, agencies, and/or subsidiaries. This can lead to
numerous Coder deployments, due to discrepancies in regulatory compliance, data
sovereignty, and level of funding across groups. The Coder Validated
Architecture (CVA) prescribes a Kubernetes-based deployment approach, enabling
your organization to deploy a stable Coder instance that is easier to maintain
and troubleshoot.

The following sections will detail the components of the Coder Validated
Architecture, provide guidance on how to configure and deploy these components,
and offer insights into how to maintain and troubleshoot your Coder environment.

- [General concepts](#general-concepts)
- [Kubernetes Infrastructure](#kubernetes-infrastructure)
- [PostgreSQL Database](#postgresql-database)
- [Operational readiness](#operational-readiness)

## Who is this document for?

This guide targets the following personas. It assumes a basic understanding of
cloud/on-premise computing, containerization, and the Coder platform.

| Role                      | Description                                                                    |
|---------------------------|--------------------------------------------------------------------------------|
| Platform Engineers        | Responsible for deploying, operating the Coder deployment and infrastructure   |
| Enterprise Architects     | Responsible for architecting Coder deployments to meet enterprise requirements |
| Managed Service Providers | Entities that deploy and run Coder software as a service for customers         |

## CVA Guidance

| CVA provides:                                  | CVA does not provide:                                                                    |
|------------------------------------------------|------------------------------------------------------------------------------------------|
| Single and multi-region K8s deployment options | Prescribing OS, or cloud vs. on-premise                                                  |
| Reference architectures for up to 3,000 users  | An approval of your architecture; the CVA solely provides recommendations and guidelines |
| Best practices for building a Coder deployment | Recommendations for every possible deployment scenario                                   |

For higher level design principles and architectural best practices, see Coder's
[Well-Architected Framework](https://coder.com/blog/coder-well-architected-framework).

## General concepts

This section outlines core concepts and terminology essential for understanding
Coder's architecture and deployment strategies.

### Administrator

An administrator is a user role within the Coder platform with elevated
privileges. Admins have access to administrative functions such as user
management, template definitions, insights, and deployment configuration.

### Coder control plane

Coder's control plane, also known as _coderd_, is the main service recommended
for deployment with multiple replicas to ensure high availability. It provides
an API for managing workspaces and templates, and serves the dashboard UI. In
addition, each _coderd_ replica hosts 3 Terraform [provisioners](#provisioner)
by default.

### User

A [user](../../users/index.md) is an individual who utilizes the Coder platform
to develop, test, and deploy applications using workspaces. Users can select
available templates to provision workspaces. They interact with Coder using the
web interface, the CLI tool, or directly calling API methods.

### Workspace

A [workspace](../../../user-guides/workspace-management.md) refers to an
isolated development environment where users can write, build, and run code.
Workspaces are fully configurable and can be tailored to specific project
requirements, providing developers with a consistent and efficient development
environment. Workspaces can be autostarted and autostopped, enabling efficient
resource management.

Users can connect to workspaces using SSH or via workspace applications like
`code-server`, facilitating collaboration and remote access. Additionally,
workspaces can be parameterized, allowing users to customize settings and
configurations based on their unique needs. Workspaces are instantiated using
Coder templates and deployed on resources created by provisioners.

### Template

A [template](../../../admin/templates/index.md) in Coder is a predefined
configuration for creating workspaces. Templates streamline the process of
workspace creation by providing pre-configured settings, tooling, and
dependencies. They are built by template administrators on top of Terraform,
allowing for efficient management of infrastructure resources. Additionally,
templates can utilize Coder modules to leverage existing features shared with
other templates, enhancing flexibility and consistency across deployments.
Templates describe provisioning rules for infrastructure resources offered by
Terraform providers.

### Workspace Proxy

A [workspace proxy](../../../admin/networking/workspace-proxies.md) serves as a
relay connection option for developers connecting to their workspace over SSH, a
workspace app, or through port forwarding. It helps reduce network latency for
geo-distributed teams by minimizing the distance network traffic needs to
travel. Notably, workspace proxies do not handle dashboard connections or API
calls.

### Provisioner

Provisioners in Coder execute Terraform during workspace and template builds.
While the platform includes built-in provisioner daemons by default, there are
advantages to employing external provisioners. These external daemons provide
secure build environments and reduce server load, improving performance and
scalability. Each provisioner can handle a single concurrent workspace build,
allowing for efficient resource allocation and workload management.

### Registry

The [Coder Registry](https://registry.coder.com) is a platform where you can
find starter templates and _Modules_ for various cloud services and platforms.

Templates help create self-service development environments using
Terraform-defined infrastructure, while _Modules_ simplify template creation by
providing common features like workspace applications, third-party integrations,
or helper scripts.

Please note that the Registry is a hosted service and isn't available for
offline use.

## Kubernetes Infrastructure

Kubernetes is the recommended, and supported platform for deploying Coder in the
enterprise. It is the hosting platform of choice for a large majority of Coder's
Fortune 500 customers, and it is the platform in which we build and test against
here at Coder.

### General recommendations

In general, it is recommended to deploy Coder into its own respective cluster,
separate from production applications. Keep in mind that Coder runs development
workloads, so the cluster should be deployed as such, without production-level
configurations.

### Compute

Deploy your Kubernetes cluster with two node groups, one for Coder's control
plane, and another for user workspaces (if you intend on leveraging K8s for
end-user compute).

#### Control plane nodes

The Coder control plane node group must be static, to prevent scale down events
from dropping pods, and thus dropping user connections to the dashboard UI and
their workspaces.

Coder's Helm Chart supports
[defining nodeSelectors, affinities, and tolerations](https://github.com/coder/coder/blob/e96652ebbcdd7554977594286b32015115c3f5b6/helm/coder/values.yaml#L221-L249)
to schedule the control plane pods on the appropriate node group.

#### Workspace nodes

Coder workspaces can be deployed either as Pods or Deployments in Kubernetes.
See our
[example Kubernetes workspace template](https://github.com/coder/coder/tree/main/examples/templates/kubernetes).
Configure the workspace node group to be auto-scaling, to dynamically allocate
compute as users start/stop workspaces at the beginning and end of their day.
Set nodeSelectors, affinities, and tolerations in Coder templates to assign
workspaces to the given node group:

```tf
resource "kubernetes_deployment" "coder" {
  spec {
    template {
      metadata {
        labels = {
          app = "coder-workspace"
        }
      }

      spec {
        affinity {
          pod_anti_affinity {
            preferred_during_scheduling_ignored_during_execution {
              weight = 1
              pod_affinity_term {
                label_selector {
                  match_expressions {
                    key      = "app.kubernetes.io/instance"
                    operator = "In"
                    values   = ["coder-workspace"]
                  }
                }
                topology_key = # add your node group label here
              }
            }
          }
        }

        tolerations {
          # Add your tolerations here
        }

        node_selector {
          # Add your node selectors here
        }

        container {
          image = "coder-workspace:latest"
          name  = "dev"
        }
      }
    }
  }
}
```

#### Node sizing

For sizing recommendations, see the below reference architectures:

- [Up to 1,000 users](1k-users.md)

- [Up to 2,000 users](2k-users.md)

- [Up to 3,000 users](3k-users.md)

### Networking

It is likely your enterprise deploys Kubernetes clusters with various networking
restrictions. With this in mind, Coder requires the following connectivity:

- Egress from workspace compute to the Coder control plane pods
- Egress from control plane pods to Coder's PostgreSQL database
- Egress from control plane pods to git and package repositories
- Ingress from user devices to the control plane Load Balancer or Ingress
  controller

We recommend configuring your network policies in accordance with the above.
Note that Coder workspaces do not require any ports to be open.

### Storage

If running Coder workspaces as Kubernetes Pods or Deployments, you will need to
assign persistent storage. We recommend leveraging a
[supported Container Storage Interface (CSI) driver](https://kubernetes-csi.github.io/docs/drivers.html)
in your cluster, with Dynamic Provisioning and read/write, to provide on-demand
storage to end-user workspaces.

The following Kubernetes volume types have been validated by Coder internally,
and/or by our customers:

- [PersistentVolumeClaim](https://kubernetes.io/docs/concepts/storage/volumes/#persistentvolumeclaim)
- [NFS](https://kubernetes.io/docs/concepts/storage/volumes/#nfs)
- [subPath](https://kubernetes.io/docs/concepts/storage/volumes/#using-subpath)
- [cephfs](https://kubernetes.io/docs/concepts/storage/volumes/#cephfs)

Our
[example Kubernetes workspace template](https://github.com/coder/coder/blob/5b9a65e5c137232351381fc337d9784bc9aeecfc/examples/templates/kubernetes/main.tf#L191-L219)
provisions a PersistentVolumeClaim block storage device, attached to the
Deployment.

It is not recommended to mount volumes from the host node(s) into workspaces,
for security and reliability purposes. The below volume types are _not_
recommended for use with Coder:

- [Local](https://kubernetes.io/docs/concepts/storage/volumes/#local)
- [hostPath](https://kubernetes.io/docs/concepts/storage/volumes/#hostpath)

Not that Coder's control plane filesystem is ephemeral, so no persistent storage
is required.

## PostgreSQL database

Coder requires access to an external PostgreSQL database to store user data,
workspace state, template files, and more. Depending on the scale of the
user-base, workspace activity, and High Availability requirements, the amount of
CPU and memory resources required by Coder's database may differ.

### Disaster recovery

Prepare internal scripts for dumping and restoring your database. We recommend
scheduling regular database backups, especially before upgrading Coder to a new
release. Coder does not support downgrades without initially restoring the
database to the prior version.

### Performance efficiency

We highly recommend deploying the PostgreSQL instance in the same region (and if
possible, same availability zone) as the Coder server to optimize for low
latency connections. We recommend keeping latency under 10ms between the Coder
server and database.

When determining scaling requirements, take into account the following
considerations:

- `2 vCPU x 8 GB RAM x 512 GB storage`: A baseline for database requirements for
  Coder deployment with less than 1000 users, and low activity level (30% active
  users). This capacity should be sufficient to support 100 external
  provisioners.
- Storage size depends on user activity, workspace builds, log verbosity,
  overhead on database encryption, etc.
- Allocate two additional CPU core to the database instance for every 1000
  active users.
- Enable High Availability mode for database engine for large scale deployments.

If you enable
[database encryption](../../../admin/security/database-encryption.md) in Coder,
consider allocating an additional CPU core to every `coderd` replica.

#### Resource utilization guidelines

Below are general recommendations for sizing your PostgreSQL instance:

- Increase number of vCPU if CPU utilization or database latency is high.
- Allocate extra memory if database performance is poor, CPU utilization is low,
  and memory utilization is high.
- Utilize faster disk options (higher IOPS) such as SSDs or NVMe drives for
  optimal performance enhancement and possibly reduce database load.

## Operational readiness

Operational readiness in Coder is about ensuring that everything is set up
correctly before launching a platform into production. It involves making sure
that the service is reliable, secure, and easily scales accordingly to user-base
needs. Operational readiness is crucial because it helps prevent issues that
could affect workspace users experience once the platform is live.

### Helm Chart Configuration

1. Reference our
   [Helm chart values file](https://github.com/coder/coder/blob/main/helm/coder/values.yaml)
   and identify the required values for deployment.
1. Create a `values.yaml` and add it to your version control system.
1. Determine the necessary environment variables. Here is the
   [full list of supported server environment variables](../../../reference/cli/server.md).
1. Follow our documented
   [steps for installing Coder via Helm](../../../install/kubernetes.md).

### Template configuration

1. Establish dedicated accounts for users with the _Template Administrator_
   role.
1. Maintain Coder templates using
   [version control](../../templates/managing-templates/change-management.md).
1. Consider implementing a GitOps workflow to automatically push new template
   versions into Coder from git. For example, on GitHub, you can use the
   [Setup Coder](https://github.com/marketplace/actions/setup-coder) action.
1. Evaluate enabling
   [automatic template updates](../../templates/managing-templates/index.md#template-update-policies)
   upon workspace startup.

### Observability

1. Enable the Prometheus endpoint (environment variable:
   `CODER_PROMETHEUS_ENABLE`).
1. Deploy the
   [Coder Observability bundle](https://github.com/coder/observability) to
   leverage pre-configured dashboards, alerts, and runbooks for monitoring
   Coder. This includes integrations between Prometheus, Grafana, Loki, and
   Alertmanager.
1. Review the [Prometheus response](../../integrations/prometheus.md) and set up
   alarms on selected metrics.

### User support

1. Incorporate [support links](../../setup/appearance.md#support-links) into
   internal documentation accessible from the user context menu. Ensure that
   hyperlinks are valid and lead to up-to-date materials.
1. Encourage the use of `coder support bundle` to allow workspace users to
   generate and provide network-related diagnostic data.
