import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import * as Mocks from "../../testHelpers/entities"
import { WorkspaceActions, WorkspaceActionsProps } from "./WorkspaceActions"

export default {
  title: "components/WorkspaceActions",
  component: WorkspaceActions,
}

const Template: Story<WorkspaceActionsProps> = (args) => (
  <WorkspaceActions {...args} />
)

const defaultArgs = {
  handleStart: action("start"),
  handleStop: action("stop"),
  handleDelete: action("delete"),
  handleUpdate: action("update"),
  handleCancel: action("cancel"),
  isOutdated: false,
  isUpdating: false,
}

export const Starting = Template.bind({})
Starting.args = {
  ...defaultArgs,
  workspaceStatus: Mocks.MockStartingWorkspace.latest_build.status,
}

export const Running = Template.bind({})
Running.args = {
  ...defaultArgs,
  workspaceStatus: Mocks.MockWorkspace.latest_build.status,
}

export const Stopping = Template.bind({})
Stopping.args = {
  ...defaultArgs,
  workspaceStatus: Mocks.MockStoppingWorkspace.latest_build.status,
}

export const Stopped = Template.bind({})
Stopped.args = {
  ...defaultArgs,
  workspaceStatus: Mocks.MockStoppedWorkspace.latest_build.status,
}

export const Canceling = Template.bind({})
Canceling.args = {
  ...defaultArgs,
  workspaceStatus: Mocks.MockCancelingWorkspace.latest_build.status,
}

export const Canceled = Template.bind({})
Canceled.args = {
  ...defaultArgs,
  workspaceStatus: Mocks.MockCanceledWorkspace.latest_build.status,
}

export const Deleting = Template.bind({})
Deleting.args = {
  ...defaultArgs,
  workspaceStatus: Mocks.MockDeletingWorkspace.latest_build.status,
}

export const Deleted = Template.bind({})
Deleted.args = {
  ...defaultArgs,
  workspaceStatus: Mocks.MockDeletedWorkspace.latest_build.status,
}

export const Outdated = Template.bind({})
Outdated.args = {
  ...defaultArgs,
  isOutdated: true,
  workspaceStatus: Mocks.MockOutdatedWorkspace.latest_build.status,
}

export const Failed = Template.bind({})
Failed.args = {
  ...defaultArgs,
  workspaceStatus: Mocks.MockFailedWorkspace.latest_build.status,
}

export const Updating = Template.bind({})
Updating.args = {
  ...defaultArgs,
  isUpdating: true,
  workspaceStatus: Mocks.MockOutdatedWorkspace.latest_build.status,
}
