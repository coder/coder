# Other JetBrains IDEs

Connect to your Coder workspace using any JetBrains IDE with remote development capabilities.

> **Popular IDEs**: For streamlined setup instructions, see the dedicated pages for [PyCharm](./pycharm.md) and [IntelliJ IDEA](./intellij.md).

## Supported IDEs

Coder supports all JetBrains IDEs with remote development capabilities:

- **CLion** (`CL`) - C/C++ development
- **GoLand** (`GO`) - Go development  
- **PhpStorm** (`PS`) - PHP development
- **Rider** (`RD`) - .NET development
- **RubyMine** (`RM`) - Ruby development
- **RustRover** (`RR`) - Rust development
- **WebStorm** (`WS`) - JavaScript/TypeScript development
- **JetBrains Fleet** - Lightweight, multi-language IDE

## Quick Start (Recommended) - 5 minutes

The easiest way to use JetBrains IDEs with Coder is through our Terraform module.

### Prerequisites

- [JetBrains Toolbox](https://www.jetbrains.com/toolbox-app/) version 2.7 or higher
- Coder version 2.24+
- Minimum 4 CPU cores and 8GB RAM (recommended by JetBrains)

### Option 1: Let Users Choose IDEs

```tf
module "jetbrains" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "1.0.1"
  agent_id = coder_agent.main.id
  folder   = "/home/coder/project"
  
  # Users can select from these options
  options = ["GO", "WS", "PS", "RD"]  # GoLand, WebStorm, PhpStorm, Rider
}
```

### Option 2: Pre-configure Specific IDEs

```tf
module "jetbrains" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "1.0.1"
  agent_id = coder_agent.main.id
  folder   = "/home/coder/project"
  
  # Always show these IDEs (no user selection)
  default = ["GO", "WS"]  # GoLand and WebStorm buttons always appear
}
```

### Option 3: All Supported IDEs

```tf
module "jetbrains" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "1.0.1"
  agent_id = coder_agent.main.id
  folder   = "/home/coder/project"
  
  # All supported IDEs (default behavior)
  options = ["CL", "GO", "PS", "RD", "RM", "RR", "WS"]
}
```

## Alternative Connection Methods

### JetBrains Gateway

JetBrains Gateway is a desktop app that connects to remote environments without downloading full IDEs.

1. **Install Gateway**: Download from [JetBrains Gateway website](https://www.jetbrains.com/remote-development/gateway/)

2. **Install Coder Plugin**:
   - Open Gateway
   - Under **Install More Providers**, find the **Coder** icon and click **Install**

3. **Connect to Workspace**:
   - Click **Connect to Coder**
   - Enter your Coder deployment URL
   - Authenticate with your Coder credentials
   - Select your workspace and preferred IDE

### JetBrains Fleet

Fleet is JetBrains' lightweight, multi-language IDE that can connect directly via SSH.

1. **Install Fleet**: Download from [JetBrains Fleet website](https://www.jetbrains.com/fleet/)

2. **Install Coder CLI** in your workspace or locally:
   ```bash
   curl -L https://coder.com/install.sh | sh
   ```

3. **Login and configure SSH**:
   ```bash
   coder login coder.example.com
   coder config-ssh
   ```

4. **Connect via SSH** in Fleet:
   - Open Fleet
   - Choose **Connect to SSH**
   - Set Host to `coder.workspace-name`
   - Fleet will connect and open your project

### Manual SSH Connection

For any JetBrains IDE with remote development support:

1. **Install your preferred IDE** locally
2. **Get SSH details** from your Coder workspace
3. **Configure remote connection** in your IDE:
   - **File** → **Remote Development** → **SSH**
   - Enter workspace connection details
   - Select project folder

## Advanced Configuration

### Language-Specific Setup

#### Go Development (GoLand)
```tf
module "jetbrains_go" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "1.0.1"
  agent_id = coder_agent.main.id
  folder   = "/workspace/go-project"
  default  = ["GO"]
}
```

#### Web Development (WebStorm)
```tf
module "jetbrains_web" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "1.0.1"
  agent_id = coder_agent.main.id
  folder   = "/workspace/web-app"
  default  = ["WS"]
}
```

#### .NET Development (Rider)
```tf
module "jetbrains_dotnet" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "1.0.1"
  agent_id = coder_agent.main.id
  folder   = "/workspace/dotnet-project"
  default  = ["RD"]
}
```

### Early Access Preview (EAP) Versions

```tf
module "jetbrains_eap" {
  count         = data.coder_workspace.me.start_count
  source        = "registry.coder.com/coder/jetbrains/coder"
  version       = "1.0.1"
  agent_id      = coder_agent.main.id
  folder        = "/home/coder/project"
  default       = ["GO", "WS"]
  channel       = "eap"      # Early Access Preview
  major_version = "2025.2"   # Specific major version
}
```

### Custom IDE Configuration

```tf
module "jetbrains_custom" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "1.0.1"
  agent_id = coder_agent.main.id
  folder   = "/workspace/project"

  ide_config = {
    "GO" = {
      name  = "GoLand (Custom)"
      icon  = "/custom/icons/goland.svg"
      build = "251.26927.50"
    }
    "WS" = {
      name  = "WebStorm (Custom)"
      icon  = "/custom/icons/webstorm.svg"
      build = "251.26927.40"
    }
  }
}
```

## Troubleshooting

### IDE Buttons Not Appearing

- Verify your template includes the JetBrains module
- Ensure Coder version is 2.24+
- Restart your workspace after template updates

### Connection Issues

- Check JetBrains Toolbox is running and updated
- Verify the specific IDE is installed through Toolbox
- Ensure workspace is running and accessible

### Performance Issues

- Verify workspace meets minimum requirements (4 CPU cores, 8GB RAM)
- Consider increasing workspace resources for better performance
- Check network connectivity between local machine and Coder

### Air-Gapped Environments

The module automatically handles air-gapped environments by falling back to pre-configured build numbers when the JetBrains API is unreachable.

## IDE Product Codes Reference

When configuring the module, use these product codes:

| IDE | Product Code | Full Name |
|-----|--------------|-----------||
| CLion | `CL` | CLion |
| GoLand | `GO` | GoLand |
| PhpStorm | `PS` | PhpStorm |
| Rider | `RD` | Rider |
| RubyMine | `RM` | RubyMine |
| RustRover | `RR` | RustRover |
| WebStorm | `WS` | WebStorm |

> [!IMPORTANT]
> Remote development works with paid and non-commercial licenses of JetBrains IDEs

<children></children>

---

**Need help?** [Create a GitHub issue](https://github.com/coder/coder/issues/new) or ask in our [Discord channel](https://discord.gg/coder).
