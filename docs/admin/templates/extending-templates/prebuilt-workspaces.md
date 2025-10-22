# Prebuilt workspaces

Prebuilt workspaces (prebuilds) reduce workspace creation time with an automatically-maintained pool of
ready-to-use workspaces for specific parameter presets.

The template administrator defines the prebuilt workspace's parameters and number of instances to keep provisioned.
The desired number of workspaces are then provisioned transparently.
When a developer creates a new workspace that matches the definition, Coder assigns them an existing prebuilt workspace.
This significantly reduces wait times, especially for templates with complex provisioning or lengthy startup procedures.

Prebuilt workspaces are:

- Created and maintained automatically by Coder to match your specified preset configurations.
- Claimed transparently when developers create workspaces.
- Monitored and replaced automatically to maintain your desired pool size.
- Automatically scaled based on time-based schedules to optimize resource usage.

Prebuilt workspaces are a special type of workspace that don't follow the
[regular workspace scheduling features](../../../user-guides/workspace-scheduling.md) like autostart and autostop. Instead, they have their own reconciliation loop that handles prebuild-specific scheduling features such as TTL and prebuild scheduling.

## Relationship to workspace presets

Prebuilt workspaces are tightly integrated with [workspace presets](./parameters.md#workspace-presets):

1. Each prebuilt workspace is associated with a specific template preset.
1. The preset must define all required parameters needed to build the workspace.
1. The preset parameters define the base configuration and are immutable once a prebuilt workspace is provisioned.
1. Parameters that are not defined in the preset can still be customized by users when they claim a workspace.
1. If a user does not select a preset but provides parameters that match one or more presets, Coder will automatically select the most specific matching preset and assign a prebuilt workspace if one is available.

## Prerequisites

- [**Premium license**](../../licensing/index.md)
- **Compatible Terraform provider**: Use `coder/coder` Terraform provider `>= 2.4.1`.

## Enable prebuilt workspaces for template presets

In your template, add a `prebuilds` block within a `coder_workspace_preset` definition to identify the number of prebuilt
instances your Coder deployment should maintain, and optionally configure a `expiration_policy` block to set a TTL
(Time To Live) for unclaimed prebuilt workspaces to ensure stale resources are automatically cleaned up.

   ```hcl
   data "coder_workspace_preset" "goland" {
     name = "GoLand: Large"
     parameters = {
       jetbrains_ide = "GO"
       cpus          = 8
       memory        = 16
     }
     prebuilds {
       instances = 3   # Number of prebuilt workspaces to maintain
       expiration_policy {
          ttl = 86400  # Time (in seconds) after which unclaimed prebuilds are expired (86400 = 1 day)
      }
     }
   }
   ```

After you publish a new template version, Coder will automatically provision and maintain prebuilt workspaces through an
internal reconciliation loop (similar to Kubernetes) to ensure the defined `instances` count are running.

The `expiration_policy` block ensures that any prebuilt workspaces left unclaimed for more than `ttl` seconds is considered
expired and automatically cleaned up.

## Prebuilt workspace lifecycle

Prebuilt workspaces follow a specific lifecycle from creation through eligibility to claiming.

1. After you configure a preset with prebuilds and publish the template, Coder provisions the prebuilt workspace(s).

   1. Coder automatically creates the defined `instances` count of prebuilt workspaces.
   1. Each new prebuilt workspace is initially owned by an unprivileged system pseudo-user named `prebuilds`.
      - The `prebuilds` user belongs to the `Everyone` group (you can add it to additional groups if needed).
   1. Each prebuilt workspace receives a randomly generated name for identification.
   1. The workspace is provisioned like a regular workspace; only its ownership distinguishes it as a prebuilt workspace.

1. Prebuilt workspaces start up and become eligible to be claimed by a developer.

   Before a prebuilt workspace is available to users:

   1. The workspace is provisioned.
   1. The agent starts up and connects to coderd.
   1. The agent starts its bootstrap procedures and completes its startup scripts.
   1. The agent reports `ready` status.

      After the agent reports `ready`, the prebuilt workspace considered eligible to be claimed.

   Prebuilt workspaces that fail during provisioning are retried with a backoff to prevent transient failures.

1. When a developer creates a new workspace, the claiming process occurs:

   1. Developer selects a template and preset that has prebuilt workspaces configured.
   1. If an eligible prebuilt workspace exists, ownership transfers from the `prebuilds` user to the requesting user.
   1. The workspace name changes to the user's requested name.
   1. `terraform apply` is executed using the new ownership details, which may affect the [`coder_workspace`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace) and
      [`coder_workspace_owner`](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/workspace_owner)
      datasources (see [Preventing resource replacement](#preventing-resource-replacement) for further considerations).

   The claiming process is transparent to the developer — the workspace will just be ready faster than usual.

You can view available prebuilt workspaces in the **Workspaces** view in the Coder dashboard:

![A prebuilt workspace in the dashboard](../../../images/admin/templates/extend-templates/prebuilt/prebuilt-workspaces.png)
_Note the search term `owner:prebuilds`._

Unclaimed prebuilt workspaces can be interacted with in the same way as any other workspace.
However, if a Prebuilt workspace is stopped, the reconciliation loop will not destroy it.
This gives template admins the ability to park problematic prebuilt workspaces in a stopped state for further investigation.

### Expiration Policy

Prebuilt workspaces support expiration policies through the `ttl` setting inside the `expiration_policy` block.
This value defines the Time To Live (TTL) of a prebuilt workspace, i.e., the duration in seconds that an unclaimed
prebuilt workspace can remain before it is considered expired and eligible for cleanup.

Expired prebuilt workspaces are removed during the reconciliation loop to avoid stale environments and resource waste.
New prebuilt workspaces are only created to maintain the desired count if needed.

### Scheduling

Prebuilt workspaces support time-based scheduling to scale the number of instances up or down.
This allows you to reduce resource costs during off-hours while maintaining availability during peak usage times.

Configure scheduling by adding a `scheduling` block within your `prebuilds` configuration:

```tf
data "coder_workspace_preset" "goland" {
   name = "GoLand: Large"
   parameters {
     jetbrains_ide = "GO"
     cpus          = 8
     memory        = 16
   }

   prebuilds {
     instances = 0                  # default to 0 instances

     scheduling {
       timezone = "UTC"             # only a single timezone may be used for simplicity

       # scale to 3 instances during the work week
       schedule {
         cron = "* 8-18 * * 1-5"    # from 8AM-6:59PM, Mon-Fri, UTC
         instances = 3              # scale to 3 instances
       }

       # scale to 1 instance on Saturdays for urgent support queries
       schedule {
         cron = "* 8-14 * * 6"      # from 8AM-2:59PM, Sat, UTC
         instances = 1              # scale to 1 instance
       }
     }
   }
}
```

**Scheduling configuration:**

- `timezone`: (Required) The timezone for all cron expressions. Only a single timezone is supported per scheduling configuration.
- `schedule`: One or more schedule blocks defining when to scale to specific instance counts.
  - `cron`: (Required) Cron expression interpreted as continuous time ranges.
  - `instances`: (Required) Number of prebuilt workspaces to maintain during this schedule.

**How scheduling works:**

1. The reconciliation loop evaluates all active schedules every reconciliation interval (`CODER_WORKSPACE_PREBUILDS_RECONCILIATION_INTERVAL`).
1. The schedule that matches the current time becomes active. Overlapping schedules are disallowed by validation rules.
1. If no schedules match the current time, the base `instances` count is used.
1. The reconciliation loop automatically creates or destroys prebuilt workspaces to match the target count.

**Cron expression format:**

Cron expressions follow the format: `* HOUR DOM MONTH DAY-OF-WEEK`

- `*` (minute): Must always be `*` to ensure the schedule covers entire hours rather than specific minute intervals
- `HOUR`: 0-23, range (e.g., 8-18 for 8AM-6:59PM), or `*`
- `DOM` (day-of-month): 1-31, range, or `*`
- `MONTH`: 1-12, range, or `*`
- `DAY-OF-WEEK`: 0-6 (Sunday=0, Saturday=6), range (e.g., 1-5 for Monday to Friday), or `*`

**Important notes about cron expressions:**

- **Minutes must always be `*`**: To ensure the schedule covers entire hours
- **Time ranges are continuous**: A range like `8-18` means from 8AM to 6:59PM (inclusive of both start and end hours)
- **Weekday ranges**: `1-5` means Monday through Friday (Monday=1, Friday=5)
- **No overlapping schedules**: The validation system prevents overlapping schedules.

**Example schedules:**

```tf
# Business hours only (8AM-6:59PM, Mon-Fri)
schedule {
  cron = "* 8-18 * * 1-5"
  instances = 5
}

# 24/7 coverage with reduced capacity overnight and on weekends
schedule {
  cron = "* 8-18 * * 1-5"  # Business hours (8AM-6:59PM, Mon-Fri)
  instances = 10
}
schedule {
  cron = "* 19-23,0-7 * * 1,5"  # Evenings and nights (7PM-11:59PM, 12AM-7:59AM, Mon-Fri)
  instances = 2
}
schedule {
  cron = "* * * * 6,0"  # Weekends
  instances = 2
}

# Weekend support (10AM-4:59PM, Sat-Sun)
schedule {
  cron = "* 10-16 * * 6,0"
  instances = 1
}
```

### Template updates and the prebuilt workspace lifecycle

Prebuilt workspaces are not updated after they are provisioned.

When a template's active version is updated:

1. Prebuilt workspaces for old versions are automatically deleted.
1. New prebuilt workspaces are created for the active template version.
1. If dependencies change (e.g., an [AMI](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/AMIs.html) update) without a template version change:
   - You can delete the existing prebuilt workspaces manually.
   - Coder will automatically create new prebuilt workspaces with the updated dependencies.

The system always maintains the desired number of prebuilt workspaces for the active template version.

## Administration and troubleshooting

### Managing resource quotas

To help prevent unexpected infrastructure costs, prebuilt workspaces can be used in conjunction with [resource quotas](../../users/quotas.md).
Because unclaimed prebuilt workspaces are owned by the `prebuilds` user, you can:

1. Configure quotas for any group that includes this user.
1. Set appropriate limits to balance prebuilt workspace availability with resource constraints.

When prebuilt workspaces are configured for an organization, Coder creates a "prebuilds" group in that organization and adds the prebuilds user to it. This group has a default quota allowance of 0, which you should adjust based on your needs:

- **Set a quota allowance** on the "prebuilds" group to control how many prebuilt workspaces can be provisioned
- **Monitor usage** to ensure the quota is appropriate for your desired number of prebuilt instances
- **Adjust as needed** based on your template costs and desired prebuilt workspace pool size

If a quota is exceeded, the prebuilt workspace will fail provisioning the same way other workspaces do.

### Managing prebuild provisioning queues

Prebuilt workspaces can overwhelm a Coder deployment, causing significant delays when users and template administrators create new workspaces or manage their templates. Fundamentally, this happens when provisioners are not able to meet the demand for provisioner jobs. Prebuilds contribute to provisioner demand by scheduling many jobs in bursts whenever templates are updated. The solution is to either increase the number of provisioners or decrease the number of requested prebuilt workspaces across the entire system.

To identify if prebuilt workspaces have overwhelmed the available provisioners in your Coder deployment, look for:

- Large or growing queue of prebuild-related jobs
- User workspace creation is slow
- Publishing a new template version is not reflected in the UI because the associated template import job has not yet finished

The troubleshooting steps below will help you assess and resolve this situation:

1) Pause prebuilt workspace reconciliation to stop the problem from getting worse
2) Check how many prebuild jobs are clogging your provisioner queue
3) Cancel excess prebuild jobs to free up provisioners for human users
4) Fix any problematic templates that are causing the issue
5) Resume prebuilt reconciliation once everything is back to normal

#### Pause prebuilds to limit potential impact

Run:

```bash
coder prebuilds pause
```

This prevents further pollution of your provisioner queues by stopping the prebuilt workspaces feature from scheduling new creation jobs. While the pause is in effect, no new prebuilt workspaces will be scheduled for any templates in any organizations across the entire Coder deployment.  Therefore, the command must be executed by a user with Owner level access. Existing prebuilt workspaces will remain in place.

**Important**: Remember to run `coder prebuilds resume` once all impact has been mitigated (see the last step in this section).

#### Assess prebuild queue impact

Next, run:

```bash
coder provisioner jobs list --status=pending --initiator=prebuilds
```

This will show a list of all pending jobs that have been enqueued by the prebuilt workspace system. The length of this list indicates whether prebuilt workspaces have overwhelmed your Coder deployment.

Human-initiated jobs have priority over pending prebuild jobs, but running prebuild jobs cannot be preempted. A long list of pending prebuild jobs increases the likelihood that all provisioners are already occupied when a user wants to create a workspace or import a new template version. This increases the likelihood that users will experience delays waiting for the next available provisioner.

#### Cancel pending prebuild jobs

Human-initiated jobs are prioritized above prebuild jobs in the provisioner queue. However, if no human-initiated jobs are queued when a provisioner becomes available, a prebuild job will occupy the provisioner. This can delay human-initiated jobs that arrive later, forcing them to wait for the next available provisioner.

To expedite fixing a broken template by ensuring maximum provisioner availability, cancel all pending prebuild jobs:

```bash
coder provisioner jobs list --status=pending --initiator=prebuilds | jq -r '.[].id' | xargs -n1 -P2 -I{} coder provisioner jobs cancel {}
```

This will clear the provisioner queue of all jobs that were not initiated by a human being, which increases the probability that a provisioner will be available when the next human operator needs it. It does not cancel running provisioner jobs, so there may still be some delay in processing new provisioner jobs until a provisioner completes its current job.

At this stage, most prebuild related impact will have been mitigated. There may still be a bugged template version, but it will no longer pollute provisioner queues with prebuilt workspace jobs. If the latest version of a template is also broken for reasons unrelated to prebuilds, then users are able to create workspaces using a previous template version. Some running jobs may have been initiated by the prebuild system, but these cannot be cancelled without potentially orphaning resources that have already been deployed by Terraform. Depending on your deployment and template provisioning times, it might be best to upload a new template version and wait for it to be processed organically.

#### Cancel running prebuild provisioning jobs (Optional)

If you need to expedite the processing of human-related jobs at the cost of some infrastructure housekeeping, you can run:

```bash
coder provisioner jobs list --status=running --initiator=prebuilds | jq -r '.[].id' | xargs -n1 -P2 -I{} coder provisioner jobs cancel {}
```

This should be done as a last resort. It will cancel running prebuild jobs (orphaning any resources that have already been deployed) and immediately make room for human-initiated jobs. Orphaned infrastructure will need to be manually cleaned up by a human operator. The process to identify and clear these orphaned resources will likely require administrative access to the infrastructure that hosts Coder workspaces. Furthermore, the ability to identify such orphaned resources will depend on metadata that should be included in the workspace template.

Once the provisioner queue has been cleared and all templates have been fixed, resume prebuild reconciliation by running:

#### Resume prebuild reconciliation

```bash
coder prebuilds resume
```

This re-enables the prebuilt workspaces feature and allows the reconciliation loop to resume normal operation. The system will begin creating new prebuilt workspaces according to your template configurations.

### Template configuration best practices

#### Preventing resource replacement

When a prebuilt workspace is claimed, another `terraform apply` run occurs with new values for the workspace owner and name.

This can cause issues in the following scenario:

1. The workspace is initially created with values from the `prebuilds` user and a random name.
1. After claiming, various workspace properties change (ownership, name, and potentially other values), which Terraform sees as configuration drift.
1. If these values are used in immutable fields, Terraform will destroy and recreate the resource, eliminating the benefit of prebuilds.

For example, when these values are used in immutable fields like the AWS instance `user_data`, you'll see resource replacement during claiming:

![Resource replacement notification](../../../images/admin/templates/extend-templates/prebuilt/replacement-notification.png)

To prevent this, add a `lifecycle` block with `ignore_changes`:

```hcl
resource "docker_container" "workspace" {
  lifecycle {
    ignore_changes = [env, image] # include all fields which caused drift
  }

  count = data.coder_workspace.me.start_count
  name  = "coder-${data.coder_workspace_owner.me.name}-${lower(data.coder_workspace.me.name)}"
  ...
}
```

Limit the scope of `ignore_changes` to include only the fields specified in the notification.
If you include too many fields, Terraform might ignore changes that wouldn't otherwise cause drift.

Learn more about `ignore_changes` in the [Terraform documentation](https://developer.hashicorp.com/terraform/language/meta-arguments#lifecycle).

_A note on "immutable" attributes: Terraform providers may specify `ForceNew` on their resources' attributes. Any change
to these attributes require the replacement (destruction and recreation) of the managed resource instance, rather than an in-place update.
For example, the [`ami`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/instance#ami-1) attribute on the `aws_instance` resource
has [`ForceNew`](https://github.com/hashicorp/terraform-provider-aws/blob/main/internal/service/ec2/ec2_instance.go#L75-L81) set,
since the AMI cannot be changed in-place._

### Preventing prebuild queue contention (recommended)

The section [Managing prebuild provisioning queues](#managing-prebuild-provisioning-queues) covers how to recover when prebuilds have already overwhelmed the provisioner queue.
This section outlines a **best-practice configuration** to prevent that situation by isolating prebuild jobs to a dedicated provisioner pool.
This setup is optional and requires minor template changes.

Coder supports [external provisioners and provisioner tags](../../provisioners/index.md), which allows you to route jobs to provisioners with matching tags.
By creating external provisioners with a special tag (e.g., `is_prebuild=true`) and updating the template to conditionally add that tag for prebuild jobs,
all prebuild work is handled by the prebuild pool.
This keeps other provisioners available to handle user-initiated jobs.

#### Setup

1. Create a provisioner key with a prebuild tag (e.g., `is_prebuild=true`).
    Provisioner keys are org-scoped and their tags are inferred automatically by provisioner daemons that use the key.
    **Note:** `coder_workspace_tags` are cumulative, so if your template already defines provisioner tags, you will need to create the provisioner key with the same tags plus the `is_prebuild=true` tag so that prebuild jobs correctly match the dedicated prebuild pool.
    See [Scoped Key](../../provisioners/index.md#scoped-key-recommended) for instructions on how to create a provisioner key.

1. Deploy a separate provisioner pool using that key (for example, via the [Helm coder-provisioner chart](https://github.com/coder/coder/pkgs/container/chart%2Fcoder-provisioner)).
    Daemons in this pool will only execute jobs that include all of the tags specified in their provisioner key.
    See [External provisioners](../../provisioners/index.md) for environment-specific deployment examples.

1. Update the template to conditionally add the prebuild tag for prebuild jobs.

    ```hcl
    data "coder_workspace_tags" "prebuilds" {
      count = data.coder_workspace_owner.me.name == "prebuilds" ? 1 : 0
      tags = {
        "is_prebuild" = "true"
      }
    }
    ```

Prebuild workspaces are a special type of workspace owned by the system user `prebuilds`.
The value `data.coder_workspace_owner.me.name` returns the name of the workspace owner, for prebuild workspaces, this value is `"prebuilds"`.
Because the condition evaluates based on the workspace owner, provisioning or deprovisioning prebuilds automatically applies the prebuild tag, whereas regular jobs (like workspace creation or template import) do not.

> [!NOTE]
> The prebuild provisioner pool can still accept non-prebuild jobs.
> To achieve a fully isolated setup, add an additional tag (`is_prebuild=false`) to your standard provisioners, ensuring a clean separation between prebuild and non-prebuild workloads.
> See [Provisioner Tags](../../provisioners/index.md#provisioner-tags) for further details.

#### Validation

To confirm that prebuild jobs are correctly routed to the new provisioner pool, use the Provisioner Jobs dashboard or the [`coder provisioner jobs list`](../../../reference/cli/provisioner_jobs_list.md) CLI command to inspect job metadata and tags.
Follow these steps:

1. Publish the new template version.

1. Validate the status of the prebuild provisioners.
    Check the Provisioners page in the Coder dashboard or run the [`coder provisioner list`](../../../reference/cli/provisioner_list.md) CLI command to ensure all prebuild provisioners are up to date and the tags are properly set.

1. Wait for the prebuilds reconciliation loop to run.
    The loop frequency is controlled by the configuration value [`CODER_WORKSPACE_PREBUILDS_RECONCILIATION_INTERVAL`](../../../reference/cli/server.md#--workspace-prebuilds-reconciliation-interval).
    When the loop runs, it will provision prebuilds for the new template version and deprovision prebuilds for the previous version.
    Both provisioning and deprovisioning jobs for prebuilds should display the tag `is_prebuild=true`.

1. Create a new workspace from a preset.
    Whether the preset uses a prebuild pool or not, the resulting job should not include the `is_prebuild=true` tag.
    This confirms that only prebuild-related jobs are routed to the dedicated prebuild provisioner pool.

### Monitoring and observability

#### Available metrics

Coder provides several metrics to monitor your prebuilt workspaces:

- `coderd_prebuilt_workspaces_created_total` (counter): Total number of prebuilt workspaces created to meet the desired instance count.
- `coderd_prebuilt_workspaces_failed_total` (counter): Total number of prebuilt workspaces that failed to build.
- `coderd_prebuilt_workspaces_claimed_total` (counter): Total number of prebuilt workspaces claimed by users.
- `coderd_prebuilt_workspaces_desired` (gauge): Target number of prebuilt workspaces that should be available.
- `coderd_prebuilt_workspaces_running` (gauge): Current number of prebuilt workspaces in a `running` state.
- `coderd_prebuilt_workspaces_eligible` (gauge): Current number of prebuilt workspaces eligible to be claimed.
- `coderd_prebuilt_workspace_claim_duration_seconds` ([_native histogram_](https://prometheus.io/docs/specs/native_histograms) support): Time to claim a prebuilt workspace from the prebuild pool.

#### Logs

Search for `coderd.prebuilds:` in your logs to track the reconciliation loop's behavior.

These logs provide information about:

1. Creation and deletion attempts for prebuilt workspaces.
1. Backoff events after failed builds.
1. Claiming operations.
