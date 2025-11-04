# Migrating Task Templates for Coder version 2.28.0

Prior to Coder version 2.28.0, the definition of a Coder task was different to the above. It required the following to be defined in the template:

1. A Coder parameter specifically named `"AI Prompt"`,
2. A `coder_workspace_app` that runs the `coder/agentapi` binary,
3. A `coder_ai_task` resource in the template that sets `sidebar_app.id`. This was generally defined in Coder modules specific to AI Tasks.

Note that 2 and 3 were generally handled by the `coder/agentapi` Terraform module.

The pre-2.28.0 definition will be supported until the release of 2.29.0. You will need to update your Tasks-enabled templates to continue using Tasks after this release.

You can view an [example migration here](https://github.com/coder/coder/pull/20420). Alternatively, follow the steps below:

## Upgrade Steps

1. Update the Coder Terraform provider to at least version 2.13.0:

```diff
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
-      version = "x.y.z"
+      version = ">= 2.13"
    }
  }
}
```

1. Define a `coder_ai_task` resource and `coder_task` data source in your template:

```diff
+data "coder_task" "me" {}
+resource "coder_ai_task" "task" {}
```

1. Update the version of the respective AI agent module (e.g. `claude-code`) to at least 4.0.0 and provide the prompt from `data.coder_task.me.prompt` instead of the "AI Prompt" parameter.

```diff
module "claude-code" {
  source              = "registry.coder.com/coder/claude-code/coder"
-  version             = "4.0.0"
+  version             = "4.0.0"
    ...
-  ai_prompt           = data.coder_parameter.ai_prompt.value
+  ai_prompt           = data.coder_task.me.prompt
}
```

1. Add the `coder_ai_task` resource and set `app_id` to the `task_app_id` output of the Claude module.

> [!NOTE] Refer to the documentation for the specific module you are using for the exact name of the output.

```diff
resource "coder_ai_task" "task" {
+ app_id = module.claude-code.task_app_id
}
```

## Coder Tasks format pre-2.28

Below is a minimal illustrative example of a Coder Tasks template pre-2.28.0:

```hcl
terraform {
  required_providers {
    coder = {
      source = "coder/coder
    }
  }
}

data "coder_workspace" "me" {}

resource "coder_agent" "main" { ... }

# The prompt is passed in via the specifically named "AI Prompt" parameter.
data "coder_parameter" "ai_prompt" {
  name    = "AI Prompt"
  mutable = true
}

# This coder_app is the interface to the Coder Task.
# This is assumed to be a running instance of coder/agentapi
resource "coder_app" "ai_agent" {
  ...
}

# Assuming that the below script runs `coder/agentapi` with the prompt
# defined in ARG_AI_PROMPT
resource "coder_script" "agentapi" {
  agent_id     = coder_agent.main.id
  run_on_start = true
  script       = <<EOT
    #!/usr/bin/env bash
    ARG_AI_PROMPT=${data.coder_parameter.ai_prompt.value} \
    /tmp/run_agentapi.sh
  EOT
  ...
}

# The coder_ai_task resource associates the task to the app.
resource "coder_ai_task" "task" {
  sidebar_app {
    id = coder_app.ai_agent.id
  }
}
```

## Tasks format post-2.28

In v2.28 and above, the following changes were made:

- The explicitly named "AI Prompt" parameter is deprecated. The task prompt is now available in the `coder_ai_task` resource (provider version 2.12 and above) and `coder_task` data source (provider version 2.13 and above).
- Modules no longer define the `coder_ai_task` resource. These must be defined explicitly in the template.
- The `sidebar_app` field of the `coder_ai_task` resource is now deprecated. In its place, use `app_id`.

Example:

```hcl
terraform {
  required_providers {
    coder = {
      source = "coder/coder
      version = ">= 2.13.0
    }
  }
}

data "coder_workspace" "me" {}

# The prompt is now available in the coder_task data source.
data "coder_task" "me" {}

resource "coder_agent" "main" { ... }

# This coder_app is the interface to the Coder Task.
# This is assumed to be a running instance of coder/agentapi (for instance, started via `coder_script`).
resource "coder_app" "ai_agent" {
  ...
}

# Assuming that the below script runs `coder/agentapi` with the prompt
# defined in ARG_AI_PROMPT
resource "coder_script" "agentapi" {
  agent_id     = coder_agent.main.id
  run_on_start = true
  script       = <<EOT
    #!/usr/bin/env bash
    ARG_AI_PROMPT=${data.coder_task.me.prompt} \
    /tmp/run_agentapi.sh
  EOT
  ...
}

# The coder_ai_task resource associates the task to the app.
resource "coder_ai_task" "task" {
  app_id = coder_app.ai_agent.id
}
```
