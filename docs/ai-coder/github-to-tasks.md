# How To: GitHub to Coder Tasks

## Background

Most software engineering organizations track and manage their codebase through GitHub, and use project mnnagement tools like Asana, Jira, or even Github's Projects to coordinate work. Across these systems, engineers are frequently performing the same repetitive workflows: triaging and addressing bugs, updating documentation, or implementing well-defined changes for example.

Coder Tasks provides a method for automating these repeatable workflows. With a Task, you can direct an agent like Claude Code to update your documentation or even diagnose and address a bug. By connecting GitHub to Coder Tasks, you can build out a GitHub workflow that will for example:

1. Trigger an automation to take a pre-existing issue
1. Automatically spin up a Coder Task with the same context from that issue and direct an agent to work on it
1. Focus on other higher-priority needs, while the agent addresses the issue
1. Get notified that the issue has been addressed, and you can review the proposed solution

This guide walks you through how to configure GitHub and Coder together so that you can tag Coder in a GitHub issue comment, and securely delegate work to coding agents in a Coder Task. 

### How Does This GHA Work

TODO implement diagram

## Implementing the GHA

The below steps outline how to use the Coder [Create Task Action GHA](https://github.com/coder/create-task-action) in a github workflow to solve a bug. The guide makes the following assumptions:

- You have access to a Coder Server that is running. If you don't have a Coder Server running, follow our [Quickstart Guide](https://coder.com/docs/tutorials/quickstart)
- Your Coder Server is accessible from GitHub
- You have an AI-enabled Task Template that can successfully create a Coder Task. If you don't have a Task Template available, follow our [Getting Started with Tasks Guide](https://coder.com/docs/ai-coder/tasks#getting-started-with-tasks)
- Check the [Requirements section of the GHA](https://github.com/coder/create-task-action?tab=readme-ov-file#requirements) for specific version requirements for your Coder deployment

This guide can be followed for other usecases beyond bugs like updating documetantion or implementing a small feature, but may require minor changes to file names and the prompts provided to the Coder Task.

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
  start-coder-task:
    runs-on: ubuntu-latest
    if: github.event.label.name == 'coder'
    steps:
      - name: Start Coder Task
        uses: coder/start-coder-task@v0.0.2
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

The prompt text can be modified to not wait for additional human input, but continue with implementing the proposed solution and creating a PR for example.

### Step 2: Setup the Required Secrets



### Step 4: Test Your Setup

The simplest way to test your new integration is to open (or create) a simple GitHub issue in your project which describes a change to your project or a relatively simple bug fix to perform. For the sake of test, we recommend something basic like “The sidebar color needs to be red” or “The text ‘Coder Tasks are Awesome’ needs to appear in the top left corner of the screen”. 

Once you identify (or create) a simple issue in GitHub issues for your repository, please label it with `coder` label. This will trigger the automatic GitHub action. You will soon receive a Tasks notification from Coder informing you of a newly started task, and you will also see the Task ID linked to the GitHub issue. 

> Why `coder` label, you might ask? Because it makes it super easy to later find all the issues or feature requests which were used with Coder Tasks.
> 

Depending on the complexity of the task and the size of your repository the Coder Task may take minutes or hours to complete. You may watch the progress by opening your task list and the details of this particular task via `https://<your.coder.server>/tasks` or by using the link which you will find in the issue you started the GitHub action from. But probably a better idea is to do something else in the meantime and just wait for the Coder Tasks notification about your task completion.

If everything went well you should also see the pull request issued on your behalf against the repository and linked to the GitHub issue you have started the whole flow from. Depending on how well the Agentic AI did its job you may also need to check the Coder Workspace assigned to this particular task and help the Agent get the changes across the finish line.

And that’s it! You may now enjoy all the hours you have saved because of this easy integration.

## Sumamry
