# Prebuilt workspaces

> [!WARNING]
> Prebuilds Compatibility Limitations:
> Prebuilt workspaces currently do not work reliably with [DevContainers feature](../managing-templates/devcontainers/index.md).
> If your project relies on DevContainer configuration, we recommend disabling prebuilds or carefully testing behavior before enabling them.
>
> We’re actively working to improve compatibility, but for now, please avoid using prebuilds with this feature to ensure stability and expected behavior.

Prebuilt workspaces allow template administrators to improve the developer experience by reducing workspace
creation time with an automatically maintained pool of ready-to-use workspaces for specific parameter presets.

The template administrator configures a template to provision prebuilt workspaces in the background, and then when a developer creates
a new workspace that matches the preset, Coder assigns them an existing prebuilt instance.
Prebuilt workspaces significantly reduce wait times, especially for templates with complex provisioning or lengthy startup procedures.

Prebuilt workspaces are:

- Created and maintained automatically by Coder to match your specified preset configurations.
- Claimed transparently when developers create workspaces.
- Monitored and replaced automatically to maintain your desired pool size.
- Automatically scaled based on time-based schedules to optimize resource usage.

## Relationship to workspace presets

Prebuilt workspaces are tightly integrated with [workspace presets](./parameters.md#workspace-presets):

1. Each prebuilt workspace is associated with a specific template preset.
1. The preset must define all required parameters needed to build the workspace.
1. The preset parameters define the base configuration and are immutable once a prebuilt workspace is provisioned.
1. Parameters that are not defined in the preset can still be customized by users when they claim a workspace.

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
          ttl = 86400  # Time (in seconds) after which unclaimed prebuilds are expired (1 day)
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

- **`timezone`**: The timezone for all cron expressions (required). Only a single timezone is supported per scheduling configuration.
- **`schedule`**: One or more schedule blocks defining when to scale to specific instance counts.
  - **`cron`**: Cron expression interpreted as continuous time ranges (required).
  - **`instances`**: Number of prebuilt workspaces to maintain during this schedule (required).

**How scheduling works:**

1. The reconciliation loop evaluates all active schedules every reconciliation interval (`CODER_WORKSPACE_PREBUILDS_RECONCILIATION_INTERVAL`).
2. The schedule that matches the current time becomes active. Overlapping schedules are disallowed by validation rules.
3. If no schedules match the current time, the base `instances` count is used.
4. The reconciliation loop automatically creates or destroys prebuilt workspaces to match the target count.

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
   - You may delete the existing prebuilt workspaces manually.
   - Coder will automatically create new prebuilt workspaces with the updated dependencies.

The system always maintains the desired number of prebuilt workspaces for the active template version.

## Administration and troubleshooting

### Managing resource quotas

Prebuilt workspaces can be used in conjunction with [resource quotas](../../users/quotas.md).
Because unclaimed prebuilt workspaces are owned by the `prebuilds` user, you can:

1. Configure quotas for any group that includes this user.
1. Set appropriate limits to balance prebuilt workspace availability with resource constraints.

If a quota is exceeded, the prebuilt workspace will fail provisioning the same way other workspaces do.

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

Learn more about `ignore_changes` in the [Terraform documentation](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle#ignore_changes).

_A note on "immutable" attributes: Terraform providers may specify `ForceNew` on their resources' attributes. Any change
to these attributes require the replacement (destruction and recreation) of the managed resource instance, rather than an in-place update.
For example, the [`ami`](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/instance#ami-1) attribute on the `aws_instance` resource
has [`ForceNew`](https://github.com/hashicorp/terraform-provider-aws/blob/main/internal/service/ec2/ec2_instance.go#L75-L81) set,
since the AMI cannot be changed in-place._

#### Updating claimed prebuilt workspace templates

Once a prebuilt workspace has been claimed, and if its template uses `ignore_changes`, users may run into an issue where the agent
does not reconnect after a template update. This shortcoming is described in [this issue](https://github.com/coder/coder/issues/17840)
and will be addressed before the next release (v2.23). In the interim, a simple workaround is to restart the workspace
when it is in this problematic state.

### Current limitations

The prebuilt workspaces feature has these current limitations:

- **Organizations**

  Prebuilt workspaces can only be used with the default organization.

  [View issue](https://github.com/coder/internal/issues/364)

### Monitoring and observability

#### Available metrics

Coder provides several metrics to monitor your prebuilt workspaces:

- `coderd_prebuilt_workspaces_created_total` (counter): Total number of prebuilt workspaces created to meet the desired instance count.
- `coderd_prebuilt_workspaces_failed_total` (counter): Total number of prebuilt workspaces that failed to build.
- `coderd_prebuilt_workspaces_claimed_total` (counter): Total number of prebuilt workspaces claimed by users.
- `coderd_prebuilt_workspaces_desired` (gauge): Target number of prebuilt workspaces that should be available.
- `coderd_prebuilt_workspaces_running` (gauge): Current number of prebuilt workspaces in a `running` state.
- `coderd_prebuilt_workspaces_eligible` (gauge): Current number of prebuilt workspaces eligible to be claimed.

#### Logs

Search for `coderd.prebuilds:` in your logs to track the reconciliation loop's behavior.

These logs provide information about:

1. Creation and deletion attempts for prebuilt workspaces.
1. Backoff events after failed builds.
1. Claiming operations.
