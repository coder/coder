# How To: GitHub to Coder Tasks

## Background

This guide will help you configure GitHub and Coder and connect them together, allowing for the Coder Tasks to be launched directly from GitHub issues and resulting in pull requests being issued from the Agentic AI against the original GitHub repository, with a link to the issue the work was triggered from. Combined with the Coder Notifications for Tasks, this setup allows for the developers to safely and securely delegate a number of implementation tasks to Agentic AI, while being able to focus on something else in the meantime. 

The diagram below illustrates the flow from GitHub to Coder Tasks and back to GitHub:

TODO

## How to implement the GHA

### Step 1: Create a Coder Deployment

Please follow [Coder Quickstart Guide](https://coder.com/docs/tutorials/quickstart) instructions or alternatively the [Coder Installation Guide](https://coder.com/docs/install) to deploy Coder to your environment. 

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
