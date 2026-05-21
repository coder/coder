# Contributing modules

Learn how to create and contribute Terraform modules to the Coder Registry. Modules provide reusable components that extend Coder workspaces with IDEs, development tools, login tools, and other features.

## What are Coder modules

Coder modules are Terraform modules that integrate with Coder workspaces to provide specific functionality. They are published to the Coder Registry at [registry.coder.com](https://registry.coder.com) and can be consumed in any Coder template using standard Terraform module syntax.

Examples of modules include:

- **Desktop IDEs**: [`jetbrains-fleet`](https://registry.coder.com/modules/coder/jetbrains-fleet), [`cursor`](https://registry.coder.com/modules/coder/cursor), [`windsurf`](https://registry.coder.com/modules/coder/windsurf), [`zed`](https://registry.coder.com/modules/coder/zed)
- **Web IDEs**: [`code-server`](https://registry.coder.com/modules/coder/code-server), [`vscode-web`](https://registry.coder.com/modules/coder/vscode-web), [`jupyter-notebook`](https://registry.coder.com/modules/coder/jupyter-notebook), [`jupyter-lab`](https://registry.coder.com/modules/coder/jupyterlab)
- **Integrations**: [`devcontainers-cli`](https://registry.coder.com/modules/coder/devcontainers-cli), [`vault-github`](https://registry.coder.com/modules/coder/vault-github), [`jfrog-oauth`](https://registry.coder.com/modules/coder/jfrog-oauth), [`jfrog-token`](https://registry.coder.com/modules/coder/jfrog-token)
- **Workspace utilities**: [`git-clone`](https://registry.coder.com/modules/coder/git-clone), [`dotfiles`](https://registry.coder.com/modules/coder/dotfiles), [`filebrowser`](https://registry.coder.com/modules/coder/filebrowser), [`coder-login`](https://registry.coder.com/modules/coder/coder-login), [`personalize`](https://registry.coder.com/modules/coder/personalize)

## Prerequisites

Before contributing modules, ensure you have:

- Basic Terraform knowledge
- [Terraform installed](https://developer.hashicorp.com/terraform/install)
- [Docker installed](https://docs.docker.com/get-docker/) (for running tests)
- [Bun installed](https://bun.sh/docs/installation) (for running tests and tooling)

## Setup your development environment

1. **Fork and clone the repository**:

   ```bash
   git clone https://github.com/your-username/registry.git
   cd registry
   ```

2. **Install dependencies**:

   ```bash
   bun install
   ```

3. **Understand the structure**:

   ```text
   registry/[namespace]/
   ├── modules/         # Your modules
   ├── .images/         # Namespace avatar and screenshots
   └── README.md        # Namespace description
   ```

## Create your first module

### 1. Set up your namespace

If you're a new contributor, create your namespace directory:

```bash
mkdir -p registry/[your-username]
mkdir -p registry/[your-username]/.images
```

Add your namespace avatar by downloading your GitHub avatar and saving it as `avatar.png`:

```bash
curl -o registry/[your-username]/.images/avatar.png https://github.com/[your-username].png
```

Create your namespace README at `registry/[your-username]/README.md`:

```markdown
---
display_name: "Your Name"
bio: "Brief description of what you do"
github: "your-username"
avatar: "./.images/avatar.png"
linkedin: "https://www.linkedin.com/in/your-username"
website: "https://your-website.com"
support_email: "support@your-domain.com"
status: "community"
---

# Your Name

Brief description of who you are and what you do.
```

> [!NOTE]
> The `linkedin`, `website`, and `support_email` fields are optional and can be omitted or left empty if not applicable.

### 2. Generate module scaffolding

Use the provided script to generate your module structure:

```bash
./scripts/new_module.sh [your-username]/[module-name]
cd registry/[your-username]/modules/[module-name]
```

This creates:

- `main.tf` - Terraform configuration template
- `README.md` - Documentation template with frontmatter
- `run.sh` - Optional execution script

### 3. Implement your module

Edit `main.tf` to build your module's features. Here's an example based on the `git-clone` module structure:

```terraform
terraform {
  required_providers {
    coder = {
      source = "coder/coder"
    }
  }
}

# Input variables
variable "agent_id" {
  description = "The ID of a Coder agent"
  type        = string
}

variable "url" {
  description = "Git repository URL to clone"
  type        = string
  validation {
    condition = can(regex("^(https?://|git@)", var.url))
    error_message = "URL must be a valid git repository URL."
  }
}

variable "base_dir" {
  description = "Directory to clone the repository into"
  type        = string
  default     = "~"
}

# Resources
resource "coder_script" "clone_repo" {
  agent_id     = var.agent_id
  display_name = "Clone Repository"
  script = <<-EOT
    #!/bin/bash
    set -e
    
    # Ensure git is installed
    if ! command -v git &> /dev/null; then
        echo "Installing git..."
        sudo apt-get update && sudo apt-get install -y git
    fi
    
    # Clone repository if it doesn't exist
    if [ ! -d "${var.base_dir}/$(basename ${var.url} .git)" ]; then
        echo "Cloning ${var.url}..."
        git clone ${var.url} ${var.base_dir}/$(basename ${var.url} .git)
    fi
  EOT
  run_on_start = true
}

# Outputs
output "repo_dir" {
  description = "Path to the cloned repository"
  value       = "${var.base_dir}/$(basename ${var.url} .git)"
}
```

### 4. Write complete tests

Create `main.test.ts` to test your module features:

```typescript
import { runTerraformApply, runTerraformInit, testRequiredVariables } from "~test"

describe("git-clone", async () => {
  await testRequiredVariables("registry/[your-username]/modules/git-clone")
  
  it("should clone repository successfully", async () => {
    await runTerraformInit("registry/[your-username]/modules/git-clone")
    await runTerraformApply("registry/[your-username]/modules/git-clone", {
      agent_id: "test-agent-id",
      url: "https://github.com/coder/coder.git",
      base_dir: "/tmp"
    })
  })
  
  it("should work with SSH URLs", async () => {
    await runTerraformInit("registry/[your-username]/modules/git-clone")
    await runTerraformApply("registry/[your-username]/modules/git-clone", {
      agent_id: "test-agent-id",
      url: "git@github.com:coder/coder.git"
    })
  })
})
```

### 5. Document your module

Update `README.md` with complete documentation:

```markdown
---
display_name: "Git Clone"
description: "Clone a Git repository into your Coder workspace"
icon: "../../../../.icons/git.svg"
verified: false
tags: ["git", "development", "vcs"]
---

# Git Clone

This module clones a Git repository into your Coder workspace and ensures Git is installed.

## Usage

```tf
module "git_clone" {
  source   = "registry.coder.com/[your-username]/git-clone/coder"
  version  = "~> 1.0"
  
  agent_id = coder_agent.main.id
  url      = "https://github.com/coder/coder.git"
  base_dir = "/home/coder/projects"
}
```

## Module best practices

### Design principles

- **Single responsibility**: Each module should have one clear purpose
- **Reusability**: Design for use across different workspace types
- **Flexibility**: Provide sensible defaults but allow customization
- **Safe to rerun**: Ensure modules can be applied multiple times safely

### Terraform conventions

- Use descriptive variable names and include descriptions
- Provide default values for optional variables
- Include helpful outputs for working with other modules
- Use proper resource dependencies
- Follow [Terraform style conventions](https://developer.hashicorp.com/terraform/language/syntax/style)

### Documentation standards

Your module README should include:

- **Frontmatter**: Required metadata for the registry
- **Description**: Clear explanation of what the module does
- **Usage example**: Working Terraform code snippet
- **Additional context**: Setup requirements, known limitations, etc.

> [!NOTE]
> Do not include variables tables in your README. The registry automatically generates variable documentation from your `main.tf` file.

## Test your module

Run tests to ensure your module works correctly:

```bash
# Test your specific module
bun test -t 'git-clone'

# Test all modules
bun test

# Format code
bun fmt
```

> [!IMPORTANT]
> Tests require Docker with `--network=host` support, which typically requires Linux. macOS users can use [Colima](https://github.com/abiosoft/colima) or [OrbStack](https://orbstack.dev/) instead of Docker Desktop.

## Contribute to existing modules

### Types of contributions

**Bug fixes**:

- Fix installation or configuration issues
- Resolve compatibility problems
- Correct documentation errors

**Feature additions**:

- Add new configuration options
- Support additional platforms or versions
- Add new features

**Maintenance**:

- Update dependencies
- Improve error handling
- Optimize performance

### Making changes

1. **Identify the issue**: Reproduce the problem or identify the improvement needed
2. **Make focused changes**: Keep modifications minimal and targeted
3. **Maintain compatibility**: Ensure existing users aren't broken
4. **Add tests**: Test new features and edge cases
5. **Update documentation**: Reflect changes in the README

### Backward compatibility

When modifying existing modules:

- Add new variables with sensible defaults
- Don't remove existing variables without a migration path
- Don't change variable types or meanings
- Test that basic configurations still work

## Versioning

When you modify a module, update its version following semantic versioning:

- **Patch** (1.0.0 → 1.0.1): Bug fixes, documentation updates
- **Minor** (1.0.0 → 1.1.0): New features, new variables
- **Major** (1.0.0 → 2.0.0): Breaking changes, removing variables

Use the version bump script to update versions:

```bash
./.github/scripts/version-bump.sh patch|minor|major
```

## Submit your contribution

1. **Create a feature branch**:

   ```bash
   git checkout -b feat/modify-git-clone-module
   ```

2. **Test thoroughly**:

   ```bash
   bun test -t 'git-clone'
   bun fmt
   ```

3. **Commit with clear messages**:

   ```bash
   git add .
   git commit -m "feat(git-clone):add git-clone module"
   ```

4. **Open a pull request**:
   - Use a descriptive title
   - Explain what the module does and why it's useful
   - Reference any related issues

## Common issues and solutions

### Testing problems

**Issue**: Tests fail with network errors
**Solution**: Ensure Docker is running with `--network=host` support

### Module development

**Issue**: Icon not displaying
**Solution**: Verify icon path is correct and file exists in `.icons/` directory

### Documentation

**Issue**: Code blocks not syntax highlighted
**Solution**: Use `tf` language identifier for Terraform code blocks

## Get help

- **Examples**: Review existing modules like [`code-server`](https://registry.coder.com/modules/coder/code-server), [`git-clone`](https://registry.coder.com/modules/coder/git-clone), and [`jetbrains`](https://registry.coder.com/modules/coder/jetbrains)
- **Issues**: Open an issue at [github.com/coder/registry](https://github.com/coder/registry/issues)
- **Community**: Join the [Coder Discord](https://discord.gg/coder) for questions
- **Documentation**: Check the [Coder docs](https://coder.com/docs) for help on Coder.

## Next steps

After creating your first module:

1. **Share with the community**: Announce your module on Discord or social media
2. **Iterate based on feedback**: Improve based on user suggestions
3. **Create more modules**: Build a collection of related tools
4. **Contribute to existing modules**: Help maintain and improve the ecosystem

Happy contributing! 🚀
