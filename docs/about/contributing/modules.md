# Coder modules

Coder modules are reusable Terraform configurations that extend workspace functionality. They provide pre-built integrations for development tools, services, and environments.

## Quick start

Add a module to your template:

```tf
module "code-server" {
  source   = "registry.coder.com/modules/code-server/coder"
  version  = "1.0.18"
  agent_id = coder_agent.example.id
}
```

Browse available modules at [registry.coder.com](https://registry.coder.com).

## How modules work

Modules use standard Terraform syntax with Coder-specific resources:

- **`coder_script`**: Runs installation and configuration scripts
- **`coder_app`**: Creates accessible applications in the workspace UI
- **`coder_env`**: Sets environment variables

Example module structure:

```tf
# Install and configure the service
resource "coder_script" "install" {
  agent_id = var.agent_id
  script   = file("${path.module}/install.sh")
}

# Make it accessible in the UI
resource "coder_app" "app" {
  agent_id     = var.agent_id
  slug         = "myapp"
  display_name = "My App"
  url          = "http://localhost:8080"
  icon         = "/icon/app.svg"
}
```

## Using modules in templates

### Basic usage

```tf
terraform {
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.0"
    }
  }
}

# Your infrastructure resources
resource "coder_agent" "main" {
  # ... agent configuration
}

# Add modules
module "code-server" {
  source   = "registry.coder.com/modules/code-server/coder"
  version  = "1.0.18"
  agent_id = coder_agent.main.id
}

module "docker" {
  source   = "registry.coder.com/modules/docker/coder"
  version  = "1.0.11"
  agent_id = coder_agent.main.id
}
```

### Configuration options

Most modules accept configuration variables:

```tf
module "code-server" {
  source   = "registry.coder.com/modules/code-server/coder"
  version  = "1.0.18"
  agent_id = coder_agent.main.id

  # Configuration
  port       = 13337
  extensions = ["ms-python.python", "golang.go"]
  folder     = "/home/coder/project"
}
```

### Template parameters

Use template parameters to make modules configurable:

```tf
data "coder_parameter" "ide" {
  name         = "ide"
  display_name = "IDE"
  description  = "Select your preferred IDE"
  default      = "code-server"

  option {
    name  = "VS Code (Web)"
    value = "code-server"
    icon  = "/icon/code.svg"
  }

  option {
    name  = "JetBrains Gateway"
    value = "jetbrains"
    icon  = "/icon/jetbrains.svg"
  }
}

module "code-server" {
  count    = data.coder_parameter.ide.value == "code-server" ? 1 : 0
  source   = "registry.coder.com/modules/code-server/coder"
  version  = "1.0.18"
  agent_id = coder_agent.main.id
}

module "jetbrains" {
  count    = data.coder_parameter.ide.value == "jetbrains" ? 1 : 0
  source   = "registry.coder.com/modules/jetbrains-gateway/coder"
  version  = "1.0.12"
  agent_id = coder_agent.main.id
}
```

## Popular modules

### Development environments

- **[code-server](https://registry.coder.com/modules/code-server)**: VS Code in the browser
- **[cursor](https://registry.coder.com/modules/cursor)**: Cursor editor with AI assistance
- **[jetbrains-gateway](https://registry.coder.com/modules/jetbrains-gateway)**: JetBrains IDEs
- **[vscode-desktop](https://registry.coder.com/modules/vscode-desktop)**: Local VS Code connection

### Development tools

- **[docker](https://registry.coder.com/modules/docker)**: Docker engine and CLI
- **[git-clone](https://registry.coder.com/modules/git-clone)**: Automatic repository cloning
- **[nodejs](https://registry.coder.com/modules/nodejs)**: Node.js runtime and npm
- **[python](https://registry.coder.com/modules/python)**: Python runtime and pip

### Services

- **[postgres](https://registry.coder.com/modules/postgres)**: PostgreSQL database
- **[redis](https://registry.coder.com/modules/redis)**: Redis cache
- **[jupyter](https://registry.coder.com/modules/jupyter)**: Jupyter notebooks

## Module registry

### Official vs community modules

- **Official modules**: `registry.coder.com/modules/{name}/coder` (maintained by Coder team)
- **Community modules**: `registry.coder.com/modules/{namespace}/{name}/coder`

### Version pinning

Always pin module versions for stability:

```tf
module "code-server" {
  source   = "registry.coder.com/modules/code-server/coder"
  version  = "1.0.18"  # Pin to specific version
  # version  = "~> 1.0" # Allow patch updates
  agent_id = coder_agent.main.id
}
```

## Creating modules

### Module structure

```text
my-module/
├── main.tf          # Terraform configuration
├── run.sh           # Installation/setup script (optional but recommended)
├── README.md        # Documentation with frontmatter
└── main.test.ts     # Test suite
```

### Basic module template

```tf
terraform {
  required_version = ">= 1.0"
  required_providers {
    coder = {
      source  = "coder/coder"
      version = ">= 2.0"
    }
  }
}

variable "agent_id" {
  type        = string
  description = "The ID of a Coder agent."
}

variable "port" {
  type        = number
  description = "Port to run the service on."
  default     = 8080
}

data "coder_workspace" "me" {}

resource "coder_script" "install" {
  agent_id     = var.agent_id
  display_name = "Install Service"
  icon         = "/icon/service.svg"
  script = templatefile("${path.module}/run.sh", {
    PORT = var.port
  })
  run_on_start = true
}

resource "coder_app" "service" {
  agent_id     = var.agent_id
  slug         = "service"
  display_name = "My Service"
  url          = "http://localhost:${var.port}"
  icon         = "/icon/service.svg"
}
```

### Required README frontmatter

```yaml
---
display_name: Module Name
description: Brief description of what the module does
icon: ../../../../.icons/service.svg
maintainer_github: your-username
verified: false
tags: [development, service]
---
```

### Testing modules

Create `main.test.ts`:

```typescript
import { describe, expect, it } from "bun:test";
import { runTerraformApply, runTerraformInit, testRequiredVariables } from "~test";

describe("my-module", async () => {
  await runTerraformInit(import.meta.dir);

  testRequiredVariables(import.meta.dir, {
    agent_id: "test-agent",
  });

  it("creates resources", async () => {
    const state = await runTerraformApply(import.meta.dir, {
      agent_id: "test-agent",
      port: 8080,
    });

    expect(state.resources).toBeDefined();
  });
});
```

## Development workflow

### Contributing to the registry

1. Fork the [registry repository](https://github.com/coder/registry)
2. Create your module in `registry/{namespace}/modules/{module-name}/`
3. Test your module: `bun test`
4. Format code: `bun run fmt`
5. Submit a pull request

### Local testing

Test modules locally in templates before publishing:

```tf
module "local-module" {
  source   = "./path/to/local/module"
  agent_id = coder_agent.main.id
}
```

## Offline installations

For air-gapped environments, modules can be vendored locally:

1. Download module source code
2. Place in your template directory
3. Reference with local path:

```tf
module "code-server" {
  source   = "./modules/code-server"
  agent_id = coder_agent.main.id
}
```

## Troubleshooting

### Common issues

**Module script failures**: Module installation or startup scripts fail during workspace creation. Check the workspace build logs and agent startup logs for specific error messages.

**Registry connection errors**: Network issues preventing module downloads from `registry.coder.com`. Ensure your Coder deployment can reach the internet or use [offline installations](#offline-installations).

**Version not found**: Specified module version doesn't exist. Check available versions at [registry.coder.com](https://registry.coder.com) or use `version = "~> 1.0"` for automatic minor updates.

**Missing agent_id**: All modules require the `agent_id` variable to attach to a workspace agent.

**Provider version conflicts**: Module requires a newer Coder provider version than your deployment. Update your Coder installation or use an older module version.

### Debugging

Check workspace build logs and startup script output:

```console
# View build logs
coder show <workspace-name> --build

# View startup script logs (from inside workspace)
cat /tmp/coder-startup-script.log

# View specific script logs
cat /tmp/coder-script-<script-id>.log
```

## Next steps

- Browse modules at [registry.coder.com](https://registry.coder.com)
- Read the [template creation guide](../../tutorials/template-from-scratch.md)
- Learn about [external authentication](../../admin/external-auth/index.md) for Git modules
