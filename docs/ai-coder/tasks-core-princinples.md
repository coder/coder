# Understanding Coder Tasks

## What is a Task? 

Tasks is Coder's platform for managing coding agents and other AI-enabled tools. With Coder Tasks, you can

- Connect an AI Agent like Claude Code or OpenAI's Codex to your IDE to assist in day-to-day development and building
- Kick off AI-enabled workflows such as upgrading a vulnerable package and automatically opening a GitHub Pull Requests with the patch
- Facilitate an automated agent to detect a failure in your CI/CD pipeline, spin up a Coder Workspace, apply a fix, and prepare a PR _without_ manual input

![Tasks UI](../images/guides/ai-agents/tasks-ui.png)Coder Tasks Dashboard view to see all available tasks.

Coder Tasks allows you and your organization to build and automate workflows to fully leverage AI. Tasks operate through Coder Workspaces, so developers can [connect via an IDE](../user-guides/workspace-access) to jump in and guide development and fully interact with the agent.

## Why Use Tasks?

- The problems they solve (consistency, reproducibility, reducing manual setup)
- How they fit into the Coder mental model
- Relation to developer productivity and environment setup

Coder Tasks make both developer-driven _and_ autonomous agentic workflows first-class citizens within your organization. Without Tasks, teams will fall back to ad-hoc scripts, one-off commands, or manual checklists to perform simpler operations that LLMs can easily automate. These work arounds can help a single engineer, but don't scale or provide consistency across an organization that is attempting to use AI as a true force multiplier. 

Tasks exist to solve these types of problems:

- **Consistency:** Capture a known, safe, & secure workflow once that can then be run anywhere
- **Reproducability:** Every Task runs from a Coder Workspace, so results are reliable
- **Productivity:** Eliminate manual processes from developer processes enabling them to focus on less defined and harder-to-do issues
- **Scalability:** Once a workflow is captured in a Task, it can be reused by other teams within your organization scaling with you as you grow
- **Flexibility:** Support both developer *AND* autonomous agentic workflows

## How to Make a Task Template
- Define what a “task template” is
- How a template differs from a single-use task
- Lifecycle: draft → reusable template

As a quick refresher, a template defines the underlying infrastructure that a Coder workspace runs on. Templates themself are writtin in Terraform managed as a `main.tf` file that defines the contents of the workspace and the resources it requires to run. You can additionally define modules and other shared resources to pull in that you want the workspace to have access to like an IDE or access to specific git repositories. 




## Task Template Design Principles
- Reusability  
- Composability (can be chained or combined)  
- Transparency (clear inputs/outputs)  
- Portability (works across environments)  

## How Tasks Fit Into Coder
- Place in the overall system (workspaces, environments, AI Coder)  
- How tasks interact with other features (e.g., provisioning, configuration)

## Best Practices for Authoring Tasks
- Keep them modular and focused
- Use clear naming and documentation
- Handle errors and edge cases gracefully

## Looking Ahead
- How tasks and templates may evolve
- Vision for tasks as part of the broader developer workflow




