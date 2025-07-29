# PyCharm

Connect to your Coder workspace using PyCharm Professional with one-click integration.

## Quick Start (Recommended) - 5 minutes

The easiest way to use PyCharm with Coder is through our Terraform module that adds PyCharm buttons directly to your workspace page.

### Prerequisites

- [JetBrains Toolbox](https://www.jetbrains.com/toolbox-app/) version 2.7 or higher installed on your local machine
- PyCharm Professional (available through Toolbox)
- Coder version 2.24+
- Minimum 4 CPU cores and 8GB RAM (recommended by JetBrains)

### Step 1: Add the Module to Your Template

Add this module to your Coder template:

```tf
module "jetbrains_pycharm" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "1.0.1"
  agent_id = coder_agent.main.id
  folder   = "/home/coder/project"
  
  # Only show PyCharm
  default = ["PY"]
}
```

### Step 2: Update Your Template

1. Push the template changes to your repository
2. Update the template in Coder
3. Restart your workspace (if already running)

### Step 3: Launch PyCharm

1. Go to your workspace page in Coder
2. Click the **PyCharm** button that now appears
3. JetBrains Toolbox will automatically:
   - Download PyCharm Professional (if not installed)
   - Connect to your workspace
   - Open your project folder

That's it! PyCharm will open with your workspace files ready for development.

## Alternative Methods

### Method 2: JetBrains Gateway Plugin

If you prefer using JetBrains Gateway directly:

1. **Install Gateway**: Download from [JetBrains Gateway website](https://www.jetbrains.com/remote-development/gateway/)

2. **Install Coder Plugin**:
   - Open Gateway
   - Go to **Install More Providers**
   - Find and install the **Coder** plugin

3. **Connect to Workspace**:
   - Click **Connect to Coder**
   - Enter your Coder deployment URL
   - Authenticate with your Coder credentials
   - Select your workspace
   - Choose PyCharm as your IDE

### Method 3: Manual SSH Connection

For advanced users who want direct SSH access:

1. **Install PyCharm Professional** locally
2. **Configure SSH Connection**:
   - Get your workspace SSH details from Coder
   - In PyCharm: **File** → **Remote Development** → **SSH**
   - Enter your workspace connection details
3. **Connect and Develop**

## Troubleshooting

### PyCharm Button Not Appearing

- Ensure your template includes the JetBrains module
- Verify Coder version is 2.24+
- Check that the workspace has been restarted after template update

### Connection Issues

- Verify JetBrains Toolbox is running and up to date
- Check your workspace is running and accessible
- Ensure PyCharm Professional is installed (Community edition doesn't support remote development)

### Performance Issues

- Verify your workspace meets the minimum requirements (4 CPU cores, 8GB RAM)
- Consider increasing workspace resources if development is slow
- Check network connectivity between your local machine and Coder

## Advanced Configuration

### Custom Project Folder

```tf
module "jetbrains_pycharm" {
  count    = data.coder_workspace.me.start_count
  source   = "registry.coder.com/coder/jetbrains/coder"
  version  = "1.0.1"
  agent_id = coder_agent.main.id
  folder   = "/workspace/my-python-project"  # Custom folder
  default  = ["PY"]
}
```

### Specific PyCharm Version

```tf
module "jetbrains_pycharm" {
  count         = data.coder_workspace.me.start_count
  source        = "registry.coder.com/coder/jetbrains/coder"
  version       = "1.0.1"
  agent_id      = coder_agent.main.id
  folder        = "/home/coder/project"
  default       = ["PY"]
  major_version = "2025.1"  # Specific version
  channel       = "release" # or "eap" for early access
}
```

## Next Steps

- [Configure your Python environment in the workspace](../../templates/)
- [Set up version control integration](../../../admin/git/)
- [Learn about Coder workspace management](../../)

---

**Need help?** [Create a GitHub issue](https://github.com/coder/coder/issues/new) or ask in our [Discord channel](https://discord.gg/coder).