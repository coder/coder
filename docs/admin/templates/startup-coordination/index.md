# Workspace startup coordination

This section helps template authors coordinate scripts that run during workspace lifecycle events.
Choose the mechanism that matches where you need to declare the dependency:

- [Declarative script ordering](./script-ordering.md) orders lifecycle-triggered `coder_script` resources in Terraform without coordination logic in script bodies.
- [`coder exp sync`](./usage.md) coordinates named units from inside running scripts when the dependency is dynamic or smaller than a whole script.

Scripts without dependencies continue to run concurrently with both mechanisms.
