# Shared Workspaces

Multiple users can securely connect to a single Coder workspace for debugging, programming, and code review. 

<!-- Insert screenshot of UI here -->

## Features

Workspace sharing is available to all Coder users by default, but platform admins with a Premium subscription can choose to disable sharing within their organizations or for their entire deployment. 

Owners of a workspace can grant access to other users or groups with scoped roles. 

This is helpful in a number of scenarios, including: 

- Developers can do ad-hoc debugging or pair programming.
- A workspace can be owned by a group of users for QA, on-call rotations, or shared staging.
- AI workflows where an agent prepares a workspace and a developer takes over to review or finalize the work (ex. with [Coder Tasks](https://coder.com/docs/ai-coder/tasks).)

## Getting Started

Workspaces can be shared through either the Coder CLI or UI. 

Before you begin, ensure that you have a version of Coder with workspace sharing enabled and that your account has permission to share workspaces. This is true by default if you are an OSS user, but Premium users are subject to organization-specific restrictions.

### CLI

To share a workspace:
- `coder sharing share <workspace> --user alice`
    - Shares the workspace with a single user, `alice`, with `use` permissions
- `coder sharing share <workspace> --user alice:admin,bob`
    - Shares the workspace with two users - `alice` with `admin` permissions, and `bob` with `use` permissions
- `coder sharing share <workspace> --group contractor`
    - Shares the workspace with `contractor`, which is a group of users

To remove sharing from a workspace:
- `coder sharing remove <workspace> --user alice`
    - Workspace is no longer shared with the user `alice`. 
- `coder sharing remove <workspace> --group contractor`
    - Workspace is no longer shared with the group `contractor`. 

To show who a workspace is shared with:
- `coder sharing show <workspace>`

To list shared workspaces: 
- `coder list --shared`
- `coder list --search shared_with_user:<user>`
- `coder list --search shared_with_group:<group>`

### UI

1. Open a workspace that you own.
2. Locate and click the 'Share' button. 

(Add screenshot of where the button is located in the UI)

3. Add the users or groups that you want to share the workspace with. For each one, select a role.
    -  `use` allows for connection via SSH and apps, the ability to start and stop the workspace, view logs and stats, and update on start when required.
    -  `admin` allows for all of the above, as well as the ability to rename the workspace, update at any time, and invite others with the `use` role.
    - Neither role allows for the user to delete the workspace.

4. Confirm changes and notify your collaborators.  

