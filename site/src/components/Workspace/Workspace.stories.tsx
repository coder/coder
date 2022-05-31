import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import * as Mocks from "../../testHelpers/entities"
import { Workspace, WorkspaceProps } from "./Workspace"

export default {
  title: "components/Workspace",
  component: Workspace,
  argTypes: {},
}

const Template: Story<WorkspaceProps> = (args) => <Workspace {...args} />

export const Started = Template.bind({})
Started.args = {
  workspace: Mocks.MockWorkspace,
  handleStart: action("start"),
  handleStop: action("stop"),
  resources: [Mocks.MockWorkspaceResource, Mocks.MockWorkspaceResource2],
  builds: [Mocks.MockWorkspaceBuild],
}

export const Starting = Template.bind({})
Starting.args = {
  ...Started.args,
  workspace: Mocks.MockStartingWorkspace,
}

export const Stopped = Template.bind({})
Stopped.args = {
  ...Started.args,
  workspace: Mocks.MockStoppedWorkspace,
}

export const Stopping = Template.bind({})
Stopping.args = {
  ...Started.args,
  workspace: Mocks.MockStoppingWorkspace,
}

export const Error = Template.bind({})
Error.args = {
  ...Started.args,
  workspace: {
    ...Mocks.MockFailedWorkspace,
    latest_build: {
      ...Mocks.MockWorkspaceBuild,
      job: {
        ...Mocks.MockProvisionerJob,
        status: "failed",
      },
      transition: "start",
    },
  },
}

export const Deleting = Template.bind({})
Deleting.args = {
  ...Started.args,
  workspace: Mocks.MockDeletingWorkspace,
}

export const Deleted = Template.bind({})
Deleted.args = {
  ...Started.args,
  workspace: Mocks.MockDeletedWorkspace,
}

export const Canceling = Template.bind({})
Canceling.args = {
  ...Started.args,
  workspace: Mocks.MockCancelingWorkspace,
}

export const Canceled = Template.bind({})
Canceled.args = {
  ...Started.args,
  workspace: Mocks.MockCanceledWorkspace,
}

export const Outdated = Template.bind({})
Outdated.args = {
  ...Started.args,
  workspace: Mocks.MockOutdatedWorkspace,
}
