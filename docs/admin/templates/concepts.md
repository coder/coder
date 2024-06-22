# Extending templates

There are a variety of Coder-native features to extend the configuration of your development environments.

<!-- TODO: May divide into sub-pages later. -->

## Workspace agents

## Resource persistence

The resources you define in a template may be _ephemeral_ or _persistent_. Persistent resources stay provisioned when workspaces are stopped, where as ephemeral resources are destroyed and recreated on restart. All resources are destroyed when a workspace is deleted.

> You can read more about how resource behavior and workspace state in the  [workspace lifecycle documentation](../workspaces/lifecycle.md).

Template resources follow the [behavior of Terraform resources](https://developer.hashicorp.com/terraform/language/resources/behavior#how-terraform-applies-a-configuration) and can be further configured  using the [lifecycle argument](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle).

### Example usage

A common configuration is a template whose only persistent resource is the home directory. This allows the developer to retain their work while ensuring the rest of their environment is consistently up-to-date on each workspace restart.


## Template variables

## Parameters

## Coder apps

### App ordering

## Agent metadata


<children>

</children>
