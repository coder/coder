import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import React from "react"
import {
  MockCanceledWorkspace,
  MockCancelingWorkspace,
  MockDeletedWorkspace,
  MockDeletingWorkspace,
  MockFailedWorkspace,
  MockOutdatedWorkspace,
  MockStartingWorkspace,
  MockStoppedWorkspace,
  MockStoppingWorkspace,
  MockWorkspace,
  MockWorkspaceBuild,
  MockWorkspaceResource,
  MockWorkspaceResource2,
} from "../../testHelpers/renderHelpers"
import { Workspace, WorkspaceProps } from "./Workspace"

export default {
  title: "components/Workspace",
  component: Workspace,
  argTypes: {},
}

const Template: Story<WorkspaceProps> = (args) => <Workspace {...args} />

export const Started = Template.bind({})
Started.args = {
  workspace: MockWorkspace,
  handleStart: action("start"),
  handleStop: action("stop"),
  resources: [MockWorkspaceResource, MockWorkspaceResource2],
  builds: [MockWorkspaceBuild],
}

export const Starting = Template.bind({})
Starting.args = { ...Started.args, workspace: MockStartingWorkspace }

export const Stopped = Template.bind({})
Stopped.args = { ...Started.args, workspace: MockStoppedWorkspace }

export const Stopping = Template.bind({})
Stopping.args = { ...Started.args, workspace: MockStoppingWorkspace }

export const Error = Template.bind({})
Error.args = { ...Started.args, workspace: MockFailedWorkspace }

export const Deleting = Template.bind({})
Deleting.args = { ...Started.args, workspace: MockDeletingWorkspace }

export const Deleted = Template.bind({})
Deleted.args = { ...Started.args, workspace: MockDeletedWorkspace }

export const Canceling = Template.bind({})
Canceling.args = { ...Started.args, workspace: MockCancelingWorkspace }

export const Canceled = Template.bind({})
Canceled.args = { ...Started.args, workspace: MockCanceledWorkspace }

export const Outdated = Template.bind({})
Outdated.args = { ...Started.args, workspace: MockOutdatedWorkspace }
