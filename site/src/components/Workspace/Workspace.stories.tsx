import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import * as Mocks from "../../testHelpers/entities"
import { Workspace, WorkspaceErrors, WorkspaceProps } from "./Workspace"

export default {
  title: "components/Workspace",
  component: Workspace,
  argTypes: {},
}

const Template: Story<WorkspaceProps> = (args) => <Workspace {...args} />

export const Running = Template.bind({})
Running.args = {
  scheduleProps: {
    onDeadlineMinus: () => {
      // do nothing, this is just for storybook
    },
    onDeadlinePlus: () => {
      // do nothing, this is just for storybook
    },
    maxDeadlineDecrease: 0,
    maxDeadlineIncrease: 24,
  },
  workspace: Mocks.MockWorkspace,
  handleStart: action("start"),
  handleStop: action("stop"),
  resources: [
    Mocks.MockWorkspaceResource,
    Mocks.MockWorkspaceResource2,
    Mocks.MockWorkspaceResource3,
  ],
  builds: [Mocks.MockWorkspaceBuild],
  canUpdateWorkspace: true,
  workspaceErrors: {},
  buildInfo: Mocks.MockBuildInfo,
  template: Mocks.MockTemplate,
}

export const WithoutUpdateAccess = Template.bind({})
WithoutUpdateAccess.args = {
  ...Running.args,
  canUpdateWorkspace: false,
}

export const Starting = Template.bind({})
Starting.args = {
  ...Running.args,
  workspace: Mocks.MockStartingWorkspace,
}

export const Stopped = Template.bind({})
Stopped.args = {
  ...Running.args,
  workspace: Mocks.MockStoppedWorkspace,
}

export const Stopping = Template.bind({})
Stopping.args = {
  ...Running.args,
  workspace: Mocks.MockStoppingWorkspace,
}

export const Failed = Template.bind({})
Failed.args = {
  ...Running.args,
  workspace: Mocks.MockFailedWorkspace,
  workspaceErrors: {
    [WorkspaceErrors.BUILD_ERROR]: Mocks.makeMockApiError({
      message: "A workspace build is already active.",
    }),
  },
}

export const Deleting = Template.bind({})
Deleting.args = {
  ...Running.args,
  workspace: Mocks.MockDeletingWorkspace,
}

export const Deleted = Template.bind({})
Deleted.args = {
  ...Running.args,
  workspace: Mocks.MockDeletedWorkspace,
}

export const Canceling = Template.bind({})
Canceling.args = {
  ...Running.args,
  workspace: Mocks.MockCancelingWorkspace,
}

export const Canceled = Template.bind({})
Canceled.args = {
  ...Running.args,
  workspace: Mocks.MockCanceledWorkspace,
}

export const Outdated = Template.bind({})
Outdated.args = {
  ...Running.args,
  workspace: Mocks.MockOutdatedWorkspace,
}

export const GetBuildsError = Template.bind({})
GetBuildsError.args = {
  ...Running.args,
  workspaceErrors: {
    [WorkspaceErrors.GET_BUILDS_ERROR]: Mocks.makeMockApiError({
      message: "There is a problem fetching builds.",
    }),
  },
}

export const GetResourcesError = Template.bind({})
GetResourcesError.args = {
  ...Running.args,
  workspaceErrors: {
    [WorkspaceErrors.GET_RESOURCES_ERROR]: Mocks.makeMockApiError({
      message: "There is a problem fetching workspace resources.",
    }),
  },
}

export const CancellationError = Template.bind({})
CancellationError.args = {
  ...Failed.args,
  workspaceErrors: {
    [WorkspaceErrors.CANCELLATION_ERROR]: Mocks.makeMockApiError({
      message: "Job could not be canceled.",
    }),
  },
}
