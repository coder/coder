# Windsurf

[Windsurf](https://codeium.com/windsurf) Codeium's code editor designed for AI-powered development. It combines JetBrains's IDE with Codeium's AI capabilities in a lightweight, browser-first experience.

## Connect to Coder via SSH

Windsurf can connect to your Coder workspaces via SSH, similar to other JetBrains products:

1. [Install Windsurf](https://www.jetbrains.com/windsurf/) on your local machine
1. Install the Coder CLI:

   <!-- copied from docs/install/cli.md - make changes there -->

   <div class="tabs">

   ### Linux/macOS

   Our install script is the fastest way to install Coder on Linux/macOS:

   ```sh
   curl -L https://coder.com/install.sh | sh
   ```

   Refer to [GitHub releases](https://github.com/coder/coder/releases) for
   alternate installation methods (e.g. standalone binaries, system packages).

   ### Windows

   Use [GitHub releases](https://github.com/coder/coder/releases) to download the
   Windows installer (`.msi`) or standalone binary (`.exe`).

   ![Windows setup wizard](../../images/install/windows-installer.png)

   Alternatively, you can use the
   [`winget`](https://learn.microsoft.com/en-us/windows/package-manager/winget/#use-winget)
   package manager to install Coder:

   ```powershell
   winget install Coder.Coder
   ```

   </div>

   Consult the [Coder CLI documentation](../../install/cli.md) for more options.

1. Log in to your Coder deployment and authenticate when prompted:

   ```shell
   coder login coder.example.com
   ```

1. Configure Coder SSH:

   ```shell
   coder config-ssh
   ```

1. Connect to your workspace in Windsurf:

   - Launch Windsurf
   - Select "Connect to Remote Host"
   - Choose "SSH" as the connection type
   - Enter "coder.workspace-name" as the host
   - Windsurf will connect to your workspace, and you can start working

## Features

Windsurf provides several notable features that work well with Coder:

- AI-powered code completion and assistance (powered by Codeium)
- Real-time collaborative editing
- Lightweight interface with fast loading times
- JetBrains code intelligence and smart navigation
- Extended language support
- Browser-based development
- Code chat and AI explanations
- Multi-cursor editing

## Web-Based Access

Windsurf is designed as a browser-first experience and can be deployed directly in your Coder workspace. There are two main approaches:

### 1. Using Windsurf as a Web Application

Your template administrator can add Windsurf as a workspace application using this Terraform configuration:

```tf
resource "coder_app" "windsurf" {
  agent_id      = coder_agent.main.id
  slug          = "windsurf"
  display_name  = "Windsurf"
  icon          = "/icon/jetbrains.svg" # Using JetBrains icon
  url           = "http://localhost:8008" # Default Windsurf port
  subdomain     = true
  share         = "authenticated"
  healthcheck {
    url       = "http://localhost:8008/healthz"
    interval  = 5
    threshold = 6
  }
}
```

### 2. Installing Windsurf in Your Workspace

You can install Windsurf directly in your workspace:

1. Add the following to your workspace startup script:

```bash
# Download and install Windsurf
curl -L "https://download.jetbrains.com/windsurf/windsurf-linux-x64.tar.gz" -o windsurf.tar.gz
mkdir -p ~/windsurf
tar -xzf windsurf.tar.gz -C ~/windsurf
rm windsurf.tar.gz

# Start Windsurf in the background
~/windsurf/bin/windsurf --port 8008 &
```

2. Configure a Coder application in your template:

```tf
resource "coder_agent" "main" {
  # ... other configuration
  startup_script = file("./startup.sh") # Include Windsurf installation
}

resource "coder_app" "windsurf" {
  agent_id      = coder_agent.main.id
  slug          = "windsurf"
  display_name  = "Windsurf"
  icon          = "/icon/jetbrains.svg"
  url           = "http://localhost:8008"
  subdomain     = true
}
```

> [!NOTE]
> The examples above use port 8008, which is common for Windsurf. Your template administrator should adjust this as needed based on your specific configuration.

## Authentication and Collaboration

Windsurf offers several options for authentication and collaboration:

### Codeium Account Integration

You can connect Windsurf to your Codeium account to enable AI features:

1. In Windsurf, click on your profile icon in the top right corner
2. Select "Sign in with Codeium"
3. Complete the authentication process
4. AI features like code completion and chat will now be available

### Collaboration Features

Windsurf excels at real-time collaboration, which works well in Coder environments:

1. To share your session with other users:
   - Click the "Collaborate" button in the toolbar
   - Generate a sharing link
   - Send the link to your collaborators

2. Collaborators can join your session and:
   - See your cursor and selections in real-time
   - Make edits that appear instantly
   - Use the built-in chat to communicate
   - View and participate in AI code completions

These collaboration features are particularly useful in Coder environments where teams already share infrastructure.

> [!NOTE]
> If you have any suggestions or experience any issues, please
> [create a GitHub issue](https://github.com/coder/coder/issues) or share in
> [our Discord channel](https://discord.gg/coder).
