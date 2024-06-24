# Extending templates

There are a variety of Coder-native features to extend the configuration of your development environments. Many of the following features are defined in your templates using the [Coder Terraform provider](https://registry.terraform.io/providers/coder/coder/latest/docs). The provider docs will provide code examples for usage; alternatively, you can view our [example templates](https://github.com/coder/coder/tree/main/examples/templates) to get started.  

<!-- TODO: Review structure

extending-templates/
README.md
- workspace agent overview
- resource persistence
- coder apps
- coder parameters
- template variables
agent-metadata.md (from old docs)
resource-metadata.md (from old docs)
resource-ordering.md (from old docs) 
-->

## Workspace agents

For users to connect to a workspace, the template must include a [`coder_agent`](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/agent). The associated agent will facilitate [workspace connections](../../../user-guides/workspace-access/README.md) via SSH, port forwarding, and IDEs. The agent may also display [workspace metadata](#agent-metadata) like resource usage. 

```hcl
resource "coder_agent" "dev" {
  os   = "linux"
  arch = "amd64"
  dir  = "/workspace"
  display_apps {
    vscode = true
  }
}
```

Templates must include some computational resource to start the agent. All processes on the workspace are then spawned from the agent. All information in the dashboard's workspace view is pulled from the agent. 

![A healthy workspace agent](../../../images/templates/healthy-workspace-agent.png)

Multiple agents may be used in a single template or even a single resource. Each agent may have it's own apps, startup script, and metadata. This can be used to associate multiple containers or VMs with a workspace. 

## Resource persistence

The resources you define in a template may be _ephemeral_ or _persistent_. Persistent resources stay provisioned when workspaces are stopped, where as ephemeral resources are destroyed and recreated on restart. All resources are destroyed when a workspace is deleted.

> You can read more about how resource behavior and workspace state in the  [workspace lifecycle documentation](../../workspaces/lifecycle.md).

Template resources follow the [behavior of Terraform resources](https://developer.hashicorp.com/terraform/language/resources/behavior#how-terraform-applies-a-configuration) and can be further configured  using the [lifecycle argument](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle).

### Example usage

A common configuration is a template whose only persistent resource is the home directory. This allows the developer to retain their work while ensuring the rest of their environment is consistently up-to-date on each workspace restart.


## Template variables

You can show live operational metrics to workspace users with agent metadata. It is the dynamic complement of resource metadata.

You specify agent metadata in the coder_agent.

## Parameters

## Coder apps

### App ordering

## Agent metadata


<children>

</children>
