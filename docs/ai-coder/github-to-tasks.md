# Guide: Create a GitHub to Coder Tasks Workflow

## Background

Most software engineering organizations track and manage their codebase through GitHub, and use project management tools like Asana, Jira, or even GitHub's Projects to coordinate work. Across these systems, engineers are frequently performing the same repetitive workflows: triaging and addressing bugs, updating documentation, or implementing well-defined changes for example.

Coder Tasks provides a method for automating these repeatable workflows. With a Task, you can direct an agent like Claude Code to update your documentation or even diagnose and address a bug. By connecting GitHub to Coder Tasks, you can build out a GitHub workflow that will for example:

1. Trigger an automation to take a pre-existing issue
1. Automatically spin up a Coder Task with the context from that issue and direct an agent to work on it
1. Focus on other higher-priority needs, while the agent addresses the issue
1. Get notified that the issue has been addressed, and you can review the proposed solution

This guide walks you through how to configure GitHub and Coder together so that you can tag Coder in a GitHub issue comment, and securely delegate work to coding agents in a Coder Task. 

## Implementing the GHA

The below steps outline how to use the Coder [Create Task Action GHA](https://github.com/coder/create-task-action) in a GitHub workflow to solve a bug. The guide makes the following assumptions:

- You have access to a Coder Server that is running. If you don't have a Coder Server running, follow our [Quickstart Guide](https://coder.com/docs/tutorials/quickstart)
- Your Coder Server is accessible from GitHub
- You have an AI-enabled Task Template that can successfully create a Coder Task. If you don't have a Task Template available, follow our [Getting Started with Tasks Guide](https://coder.com/docs/ai-coder/tasks#getting-started-with-tasks)
- Check the [Requirements section of the GHA](https://github.com/coder/create-task-action?tab=readme-ov-file#requirements) for specific version requirements for your Coder deployment and the following
  - GitHub OAuth is configured in your Coder Deployment
  - Users have linked their GitHub account to Coder via `/settings/external-auth`
  

This guide can be followed for other use cases beyond bugs like updating documentation or implementing a small feature, but may require minor changes to file names and the prompts provided to the Coder Task.

### Step 1: Create a GitHub Workflow file

In your repository, create a new file in the `./.github/workflows/` directory named `triage-bug.yaml`. Within that file, add the following code: 

```yaml
name: Start Coder Task

on:
  issues:
    types:
      - labeled

permissions:
  issues: write

jobs:
  coder-create-task:
    runs-on: ubuntu-latest
    if: github.event.label.name == 'coder'
    steps:
      - name: Coder Create Task
        uses: coder/create-coder-task@v0
        with:
          coder-url: ${{ secrets.CODER_URL }}
          coder-token: ${{ secrets.CODER_TOKEN }}
          coder-organization: "default"
          coder-template-name: "my-template"
          coder-task-name-prefix: "gh-task"
          coder-task-prompt: "Use the gh CLI to read ${{ github.event.issue.html_url }}, write an appropriate plan for solving the issue to PLAN.md, and then wait for feedback."
          github-user-id: ${{ github.event.sender.id }}
          github-issue-url: ${{ github.event.issue.html_url }}
          github-token: ${{ github.token }}
          comment-on-issue: true
```

This code will perform the following actions:

- Create a Coder Task when you apply the `coder` label to an existing GitHub issue
- Pass as a prompt to the Coder Task
    
    1. Use the GitHub CLI to access and read the content of the linked GitHub issue
    1. Generate an initial implementation plan to solve the bug
    1. Write that plan to a `PLAN.md` file
    1. Wait for additional input

- Post an update on the GitHub ticket with a link to the task

The prompt text can be modified to not wait for additional human input, but continue with implementing the proposed solution and creating a PR for example. Note that this example prompt uses the GitHub CLI `gh`, which must be installed in your Coder template. The CLI will automatically authenticate using the user's linked GitHub account via Coder's external auth.

### Step 2: Setup the Required Secrets & Inputs

The GHA has multiple required inputs that require configuring before the workflow can successfully operate. 

You must set the following inputs as secrets within your repository:

- `coder-url`: the URL of your Coder deployment, e.g. https://coder.example.com
- `coder-token`: follow our [API Tokens documentation](https://coder.com/docs/admin/users/sessions-tokens#long-lived-tokens-api-tokens) to generate a token. Note that the token must be an admin/org-level with the "Read users in organization" and "Create tasks for any user" permissions

You must also set `coder-template-name` as part of this. The GHA example has this listed as a secret, but the value doesn't need to be stored as a secret. The template name can be determined the following ways:

- By viewing the URL of the template in the UI, e.g. `https://your-coder-url/templates/<org-name>/<template-name>`
- Using the Coder CLI:

```bash
# List all templates in your organization
coder templates list

# List templates in a specific organization
coder templates list --org your-org-name
```

You can also choose to modify the other [input parameters](https://github.com/coder/create-task-action?tab=readme-ov-file#inputs) to better fit your desired workflow.

### Step 3: Test Your Setup

Create a new GitHub issue for a bug in your codebase. We recommend a basic bug, for this test, like “The sidebar color needs to be red” or “The text ‘Coder Tasks are Awesome’ needs to appear in the top left corner of the screen”. You should adapt the phrasing to be specific to your codebase.

Add the `coder` label to that GitHub issue. You should see the following things occur:

- A comment is made on the issue saying `Task created: https://your-coder-url/tasks/username/task-id`
- A Coder Task will spin up, and you'll receive a Tasks notification to that effect
- You can click the link to follow the Task's progress in creating a plan to solve your bug

Depending on the complexity of the task and the size of your repository, the Coder Task may take minutes or hours to complete. Our recommendation is to rely on Task Notifications to know when the Task completes, and further action is required.

And that’s it! You may now enjoy all the hours you have saved because of this easy integration.

#### Step 4: Adapt this Workflow to your Processes

Following the above steps sets up a GitHub Workflow that will

1. Allow you to label bugs with `coder`
1. A coding agent will determine a plan to address the bug
1. You'll receive a notification to review the plan and prompt the agent to proceed, or change course

We recommend that you further adapt this workflow to better match your process. For example, you could:

- Modify the prompt to implement the plan it came up with, and then create a PR once it has a solution
- Update your GitHub issue template to automatically apply the `coder` label to attempt to solve bugs that have been logged
- Modify the underlying use case to handle updating documentation, implementing a small feature, reviewing bug reports for completeness, or even writing unit test
- Modify the workflow trigger for other scenarios such as:

```yml
# Comment-based trigger slash commands
on:
  issue_comment:
    types: [created]

jobs:
  trigger-on-comment:
    runs-on: ubuntu-latest
    if: startsWith(github.event.comment.body, '/coder')

# On Pull Request Creation
jobs:
  on-pr-opened:
    runs-on: ubuntu-latest
    # No if needed - just runs on PR open

# On changes to a specific directory
on:
  pull_request:
    paths:
      - 'docs/**'
      - 'src/api/**'
      - '*.md'

jobs:
  on-docs-changed:
    runs-on: ubuntu-latest
    # Runs automatically when files in these paths change
```

## Summary

This guide shows you how to automatically delegate routine engineering work to AI coding agents by connecting GitHub issues to Coder Tasks. When you label an issue (like a bug report or documentation update), a coding agent spins up in a secure Coder workspace, reads the issue context, and works on solving it while you focus on higher-priority tasks. The agent reports back with a proposed solution for you to review and approve, turning hours of repetitive work into minutes of oversight. This same pattern can be adapted to handle documentation updates, test writing, code reviews, and other automatable workflows across your development process.

## Troubleshooting

### "No Coder user found with GitHub user ID X"

**Cause:** The user who triggered the workflow hasn't linked their GitHub account to Coder.

**Solution:** 
1. Ensure GitHub OAuth is configured in your Coder deployment (see [External Authentication docs](https://coder.com/docs/admin/external-auth#configure-a-github-oauth-app))
2. Have the user visit `https://your-coder-url/settings/external-auth` and link their GitHub account
3. Retry the workflow by re-applying the `coder` label or however else the workflow is triggered

### "Failed to create task: 403 Forbidden"

**Cause:** The `coder-token` doesn't have the required permissions.

**Solution:** The token must have:
- Read users in organization
- Create tasks for any user

Generate a new token with these permissions at `https://your-coder-url/deployment/general`. See the [Coder Create Task GHA requirements](https://github.com/coder/create-task-action?tab=readme-ov-file#requirements) for more specific information.

### "Template 'my-template' not found"

**Cause:** The `coder-template-name` is incorrect or the template doesn't exist in the specified organization.

**Solution:**
1. Verify the template name using: `coder templates list --org your-org-name`
2. Update the `coder-template-name` input in your workflow file to match exactly, or input secret or variable saved in GitHub
3. Ensure the template exists in the organization specified by `coder-organization`
