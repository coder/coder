# GitHub Actions Integration

Coder provides a GitHub Action that automatically starts workspaces from GitHub issues and comments, enabling seamless integration between your development workflow and AI coding agents.

## Start Workspace Action

The [start-workspace-action](https://github.com/coder/start-workspace-action) creates Coder workspaces triggered by GitHub events and posts status updates directly to GitHub issues.

### Features

- **Automatic workspace creation** from GitHub issues or comments
- **Real-time status updates** posted as GitHub issue comments
- **User mapping** between GitHub and Coder accounts
- **Configurable triggers** based on your workflow requirements
- **Template parameters** for customizing workspace environments

### Basic Usage

Here's an example workflow that starts a workspace when issues are created or when comments contain "@coder":

```yaml
name: Start Workspace On Issue Creation or Comment

on:
  issues:
    types: [opened]
  issue_comment:
    types: [created]

permissions:
  issues: write

jobs:
  start-workspace:
    runs-on: ubuntu-latest
    if: >
      (github.event_name == 'issue_comment' && contains(github.event.comment.body, '@coder')) || 
      (github.event_name == 'issues' && contains(github.event.issue.body, '@coder'))
    environment: coder-production
    timeout-minutes: 5
    steps:
      - name: Start Coder workspace
        uses: coder/start-workspace-action@v0.1.0
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          github-username: >
            ${{ 
              (github.event_name == 'issue_comment' && github.event.comment.user.login) || 
              (github.event_name == 'issues' && github.event.issue.user.login)
            }}
          coder-url: ${{ secrets.CODER_URL }}
          coder-token: ${{ secrets.CODER_TOKEN }}
          template-name: ${{ secrets.CODER_TEMPLATE_NAME }}
          parameters: |-
            Coder Image: codercom/oss-dogfood:latest
            Coder Repository Base Directory: "~"
            AI Code Prompt: "Use the gh CLI tool to read the details of issue https://github.com/${{ github.repository }}/issues/${{ github.event.issue.number }} and then address it."
            Region: us-pittsburgh
```

### Configuration

#### Required Inputs

| Input | Description |
|-------|-------------|
| `coder-url` | Your Coder deployment URL |
| `coder-token` | API token for Coder (requires admin privileges) |
| `template-name` | Name of the Coder template to use |
| `parameters` | YAML-formatted parameters for the workspace |

#### Optional Inputs

| Input | Description | Default |
|-------|-------------|----------|
| `github-token` | GitHub token for posting comments | `${{ github.token }}` |
| `github-issue-number` | Issue number for status comments | Current issue from context |
| `github-username` | GitHub user to map to Coder user | - |
| `coder-username` | Coder username (alternative to github-username) | - |
| `workspace-name` | Name for the new workspace | `issue-{issue_number}` |

### User Mapping

The action supports two methods for mapping users:

1. **GitHub Username Mapping** (Coder 2.21+): Set `github-username` to automatically map GitHub users to Coder users who have logged in with the same GitHub account.

2. **Direct Coder Username**: Set `coder-username` to specify the exact Coder user.

### Security Best Practices

Since this action requires a Coder admin API token, follow these security recommendations:

1. **Use GitHub Environments**: Store sensitive secrets in a GitHub environment (e.g., "coder-production")
2. **Restrict Branch Access**: Limit the environment to specific branches (e.g., main)
3. **Minimal Permissions**: Use the least privileged token possible

```yaml
jobs:
  start-workspace:
    runs-on: ubuntu-latest
    # Restrict access to secrets using environments
    environment: coder-production
    steps:
      - name: Start Coder workspace
        uses: coder/start-workspace-action@v0.1.0
        with:
          coder-token: ${{ secrets.CODER_TOKEN }}
          # other inputs...
```

### AI Agent Integration

This action works particularly well with AI coding agents by:

- **Providing context**: Pass issue details to agents via template parameters
- **Automating setup**: Pre-configure workspaces with necessary tools and repositories
- **Enabling collaboration**: Allow agents to work on issues triggered by team members

#### Example AI Agent Prompt

```yaml
parameters: |-
  AI Code Prompt: |
    You are an AI coding assistant. Your task is to:
    1. Read the GitHub issue at https://github.com/${{ github.repository }}/issues/${{ github.event.issue.number }}
    2. Analyze the requirements and existing codebase
    3. Implement the requested changes
    4. Run tests to ensure functionality
    5. Create a pull request with your solution
```

### Workflow Examples

#### Issue-Triggered Workspaces

```yaml
on:
  issues:
    types: [opened, labeled]

jobs:
  start-workspace:
    if: contains(github.event.issue.labels.*.name, 'ai-assist')
    # ... rest of configuration
```

#### Comment-Triggered Workspaces

```yaml
on:
  issue_comment:
    types: [created]

jobs:
  start-workspace:
    if: |
      github.event.issue.pull_request == null &&
      contains(github.event.comment.body, '/coder start')
    # ... rest of configuration
```

### Requirements

- Coder deployment with API access
- Coder 2.21+ for GitHub username mapping (earlier versions can use `coder-username`)
- GitHub repository with appropriate secrets configured
- Coder template configured for AI agent workflows

### Troubleshooting

#### Common Issues

- **Authentication failures**: Ensure `CODER_TOKEN` has admin privileges
- **User mapping errors**: Verify GitHub users have logged into Coder with the same account
- **Template not found**: Check that `template-name` exists and is accessible
- **Parameter validation**: Ensure template parameters match expected format

#### GitHub Enterprise

This action supports GitHub Enterprise with the exception of the `github-username` input. Use `coder-username` instead for GitHub Enterprise deployments.

## Next Steps

- [Configure Coder Tasks](./tasks.md) for running AI agents
- [Set up custom agents](./custom-agents.md) in your templates
- [Review security considerations](./security.md) for AI agent deployments
