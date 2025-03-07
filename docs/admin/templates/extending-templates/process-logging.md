# Workspace Process Logging

The workspace process logging feature allows you to log all system-level
processes executing in the workspace.

> [!NOTE]
> This feature is only available on Linux in Kubernetes. There are
> additional requirements outlined further in this document.

Workspace process logging adds a sidecar container to workspace pods that will
log all processes started in the workspace container (e.g., commands executed in
the terminal or processes created in the background by other processes).
Processes launched inside containers or nested containers within the workspace
are also logged. You can view the output from the sidecar or send it to a
monitoring stack, such as CloudWatch, for further analysis or long-term storage.

Please note that these logs are not recorded or captured by the Coder
organization in any way, shape, or form.

> This is an [Premium or Enterprise](https://coder.com/pricing) feature. To
> learn more about Coder licensing, please
> [contact sales](https://coder.com/contact).

## How this works

Coder uses [eBPF](https://ebpf.io/) (which we chose for its minimal performance
impact) to perform in-kernel logging and filtering of all exec system calls
originating from the workspace container.

The core of this feature is also open source and can be found in the
[exectrace](https://github.com/coder/exectrace) GitHub repo. The enterprise
component (in the `enterprise/` directory of the repo) is responsible for
starting the eBPF program with the correct filtering options for the specific
workspace.

## Requirements

The host machine must be running a Linux kernel >= 5.8 with the kernel config
`CONFIG_DEBUG_INFO_BTF=y` enabled.

To check your kernel version, run:

```shell
uname -r
```

To validate the required kernel config is enabled, run either of the following
commands on your nodes directly (_not_ from a workspace terminal):

```shell
cat /proc/config.gz | gunzip | grep CONFIG_DEBUG_INFO_BTF
```

```shell
cat "/boot/config-$(uname -r)" | grep CONFIG_DEBUG_INFO_BTF
```

If these requirements are not met, workspaces will fail to start for security
reasons.

Your template must be a Kubernetes template. Workspace process logging is not
compatible with the `sysbox-runc` runtime due to technical limitations, but it
is compatible with our `envbox` template family.

## Example templates

We provide working example templates for Kubernetes, and Kubernetes with
`envbox` (for [Docker support in workspaces](./docker-in-workspaces.md)). You
can view these templates in the
[exectrace repo](https://github.com/coder/exectrace/tree/main/enterprise/templates).

## Configuring custom templates to use workspace process logging

If you have an existing Kubernetes or Kubernetes with `envbox` template that you
would like to add workspace process logging to, follow these steps:

1. Ensure the image used in your template has `curl` installed.

1. Add the following section to your template's `main.tf` file:

   <!--
     If you are updating this section, please also update the example templates
     in the exectrace repo.
   -->

   ```hcl
   locals {
     # This is the init script for the main workspace container that runs before the
     # agent starts to configure workspace process logging.
     exectrace_init_script = <<EOT
       set -eu
       pidns_inum=$(readlink /proc/self/ns/pid | sed 's/[^0-9]//g')
       if [ -z "$pidns_inum" ]; then
         echo "Could not determine process ID namespace inum"
         exit 1
       fi

       # Before we start the script, does curl exist?
       if ! command -v curl >/dev/null 2>&1; then
         echo "curl is required to download the Coder binary"
         echo "Please install curl to your image and try again"
         # 127 is command not found.
         exit 127
       fi

       echo "Sending process ID namespace inum to exectrace sidecar"
       rc=0
       max_retry=5
       counter=0
       until [ $counter -ge $max_retry ]; do
         set +e
         curl \
           --fail \
           --silent \
           --connect-timeout 5 \
           -X POST \
           -H "Content-Type: text/plain" \
           --data "$pidns_inum" \
           http://127.0.0.1:56123
         rc=$?
         set -e
         if [ $rc -eq 0 ]; then
           break
         fi

         counter=$((counter+1))
         echo "Curl failed with exit code $${rc}, attempt $${counter}/$${max_retry}; Retrying in 3 seconds..."
         sleep 3
       done
       if [ $rc -ne 0 ]; then
         echo "Failed to send process ID namespace inum to exectrace sidecar"
         exit $rc
       fi

     EOT
   }
   ```

1. Update the `command` of your workspace container like the following:

   <!--
     If you are updating this section, please also update the example templates
     in the exectrace repo.
   -->

   ```hcl
   resource "kubernetes_pod" "main" {
     ...
     spec {
       ...
       container {
         ...
         // NOTE: this command is changed compared to the upstream kubernetes
         // template
         command = [
           "sh",
           "-c",
           "${local.exectrace_init_script}\n\n${coder_agent.main.init_script}",
         ]
         ...
       }
       ...
     }
     ...
   }
   ```

   > [!NOTE]
   > If you are using the `envbox` template, you will need to update
   > the third argument to be
   > `"${local.exectrace_init_script}\n\nexec /envbox docker"` instead.

1. Add the following container to your workspace pod spec.

   <!--
     If you are updating this section, please also update the example templates
     in the exectrace repo.
   -->

   ```hcl
   resource "kubernetes_pod" "main" {
     ...
     spec {
       ...
       // NOTE: this container is added compared to the upstream kubernetes
       // template
       container {
         name              = "exectrace"
         image             = "ghcr.io/coder/exectrace:latest"
         image_pull_policy = "Always"
         command = [
           "/opt/exectrace",
           "--init-address", "127.0.0.1:56123",
           "--label", "workspace_id=${data.coder_workspace.me.id}",
           "--label", "workspace_name=${data.coder_workspace.me.name}",
           "--label", "user_id=${data.coder_workspace_owner.me.id}",
           "--label", "username=${data.coder_workspace_owner.me.name}",
           "--label", "user_email=${data.coder_workspace_owner.me.email}",
         ]
         security_context {
           // exectrace must be started as root so it can attach probes into the
           // kernel to record process events with high throughput.
           run_as_user  = "0"
           run_as_group = "0"
           // exectrace requires a privileged container so it can control mounts
           // and perform privileged syscalls against the host kernel to attach
           // probes.
           privileged = true
         }
       }
       ...
     }
     ...
   }
   ```

   > [!NOTE]
   > `exectrace` requires root privileges and a privileged container
   > to attach probes to the kernel. This is a requirement of eBPF.

1. Add the following environment variable to your workspace pod:

   <!--
     If you are updating this section, please also update the example templates
     in the exectrace repo.
   -->

   ```hcl
   resource "kubernetes_pod" "main" {
     ...
     spec {
       ...
       env {
         name = "CODER_AGENT_SUBSYSTEM"
         value = "exectrace"
       }
       ...
     }
     ...
   }
   ```

Once you have made these changes, you can push a new version of your template
and workspace process logging will be enabled for all workspaces once they are
restarted.

## Viewing workspace process logs

To view the process logs for a specific workspace you can use `kubectl` to print
the logs:

```bash
kubectl logs pod-name --container exectrace
```

The raw logs will look something like this:

```json
{
    "ts": "2022-02-28T20:29:38.038452202Z",
    "level": "INFO",
    "msg": "exec",
    "fields": {
        "labels": {
            "user_email": "jessie@coder.com",
            "user_id": "5e876e9a-121663f01ebd1522060d5270",
            "username": "jessie",
            "workspace_id": "621d2e52-a6987ef6c56210058ee2593c",
            "workspace_name": "main"
        },
        "cmdline": "uname -a",
        "event": {
            "filename": "/usr/bin/uname",
            "argv": ["uname", "-a"],
            "truncated": false,
            "pid": 920684,
            "uid": 101000,
            "gid": 101000,
            "comm": "bash"
        }
    }
}
```

### View logs in AWS EKS

If you're using AWS' Elastic Kubernetes Service, you can
[configure your cluster](https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/Container-Insights-EKS-logs.html)
to send logs to CloudWatch. This allows you to view the logs for a specific user
or workspace.

To view your logs, go to the CloudWatch dashboard (which is available on the
**Log Insights** tab) and run a query similar to the following:

```text
fields @timestamp, log_processed.fields.cmdline
| sort @timestamp asc
| filter kubernetes.container_name="exectrace"
| filter log_processed.fields.labels.username="zac"
| filter log_processed.fields.labels.workspace_name="code"
```

## Usage considerations

- The sidecar attached to each workspace is a privileged container, so you may
  need to review your organization's security policies before enabling this
  feature. Enabling workspace process logging does _not_ grant extra privileges
  to the workspace container itself, however.
- `exectrace` will log processes from nested Docker containers (including deeply
  nested containers) correctly, but Coder does not distinguish between processes
  started in the workspace and processes started in a child container in the
  logs.
- With `envbox` workspaces, this feature will detect and log startup processes
  begun in the outer container (including container initialization processes).
- Because this feature logs **all** processes in the workspace, high levels of
  usage (e.g., during a `make` run) will result in an abundance of output in the
  sidecar container. Depending on how your Kubernetes cluster is configured, you
  may incur extra charges from your cloud provider to store the additional logs.
