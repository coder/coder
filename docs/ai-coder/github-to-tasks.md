# How To: GitHub to Coder Tasks

## Background

Most software engineering organizations track and manage their codebase through GitHub, and use project mnnagement tools like Asana, Jira, or even Github's Projects to coordinate work. Across these systems, engineers are frequently performing the same repetitive workflows: triaging and addressing bugs, updating documentation, or implementing well-defined changes for example.

Coder Tasks provides a method for automating these repeatable workflows. With a Task, you can direct an agent like Claude Code to update your documentation or even diagnose and address a bug. By connecting GitHub to Coder Tasks, you can build out a GitHub workflow that will for example:

1. Trigger an automation to take a pre-existing issue
1. Automatically spin up a Coder Task with the same context from that issue and direct an agent to work on it
1. Focus on other higher-priority needs, while the agent addresses the issue
1. Get notified that the issue has been addressed, and you can review the proposed solution

This guide walks you through how to configure GitHub and Coder together so that you can tag Coder in a GitHub issue comment, and securely delegate work to coding agents.

### How Does This GHa Work

TODO implement diagram

## How to implement the GHA

### Step 1: Create a Coder Deployment

Follow the [Coder Quickstart Guide](https://coder.com/docs/tutorials/quickstart) or the [Coder Installation Guide](https://coder.com/docs/install) to deploy Coder to your environment. 

In order for the GitHub to Coder integration to work, your Coder server needs to be accessible by GitHub. You will need to connect Coder to GitHub in the future steps.

### Step 2: Enable Claude Code in Coder Tasks

In this step you need to configure your Coder Tasks template with Claude Code (or any other Agentic AI of your choice). This will allow you to use this template for the purpose of the integration.

It is strongly advised that you used the following Template Example for your setup. This template (**link needed**) has been tuned specifically for this type of integrations by Coder experts. 

If you already have a polished template which you are using in your environment, please merge the blocks labelled in the example (**link needed**) with `COPY THIS TO YOUR TEMPLATE` comment. These sections are coming with additional comments which will help you understand what they do. 

Of course, you may always tune the Template further if you want to. Please check what we have in [Coder Registry](https://registry.coder.com/) to enhance it with additional modules.

### Step 3: Create the GitHub Workflow

In this step you need to configure your Coder Tasks template with Claude Code (or any other Agentic AI of your choice). This will allow you to use this template for the purpose of the integration.

It is strongly advised that you used the following Template Example for your setup. This template (**link needed**) has been tuned specifically for this type of integrations by Coder experts. 

If you already have a polished template which you are using in your environment, please merge the blocks labelled in the example (**link needed**) with `COPY THIS TO YOUR TEMPLATE` comment. These sections are coming with additional comments which will help you understand what they do. 

Of course, you may always tune the Template further if you want to. Please check what we have in [Coder Registry](https://registry.coder.com/) to enhance it with additional modules.

### Step 4: Test Your Setup

The simplest way to test your new integration is to open (or create) a simple GitHub issue in your project which describes a change to your project or a relatively simple bug fix to perform. For the sake of test, we recommend something basic like “The sidebar color needs to be red” or “The text ‘Coder Tasks are Awesome’ needs to appear in the top left corner of the screen”. 

Once you identify (or create) a simple issue in GitHub issues for your repository, please label it with `coder` label. This will trigger the automatic GitHub action. You will soon receive a Tasks notification from Coder informing you of a newly started task, and you will also see the Task ID linked to the GitHub issue. 

> Why `coder` label, you might ask? Because it makes it super easy to later find all the issues or feature requests which were used with Coder Tasks.
> 

Depending on the complexity of the task and the size of your repository the Coder Task may take minutes or hours to complete. You may watch the progress by opening your task list and the details of this particular task via `https://<your.coder.server>/tasks` or by using the link which you will find in the issue you started the GitHub action from. But probably a better idea is to do something else in the meantime and just wait for the Coder Tasks notification about your task completion.

If everything went well you should also see the pull request issued on your behalf against the repository and linked to the GitHub issue you have started the whole flow from. Depending on how well the Agentic AI did its job you may also need to check the Coder Workspace assigned to this particular task and help the Agent get the changes across the finish line.

And that’s it! You may now enjoy all the hours you have saved because of this easy integration.

## Sumamry
