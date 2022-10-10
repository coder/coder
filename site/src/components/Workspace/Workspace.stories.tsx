import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import dayjs from "dayjs"
import { canExtendDeadline, canReduceDeadline } from "util/schedule"
import * as Mocks from "../../testHelpers/entities"
import { Workspace, WorkspaceErrors, WorkspaceProps } from "./Workspace"

export default {
  title: "components/Workspace",
  component: Workspace,
  argTypes: {},
}

const Template: Story<WorkspaceProps> = (args) => <Workspace {...args} />

export const Started = Template.bind({})
Started.args = {
  bannerProps: {
    isLoading: false,
    onExtend: action("extend"),
  },
  scheduleProps: {
    onDeadlineMinus: () => {
      // do nothing, this is just for storybook
    },
    onDeadlinePlus: () => {
      // do nothing, this is just for storybook
    },
    deadlineMinusEnabled: () => {
      return canReduceDeadline(dayjs(Mocks.MockWorkspace.latest_build.deadline))
    },
    deadlinePlusEnabled: () => {
      return canExtendDeadline(
        dayjs(Mocks.MockWorkspace.latest_build.deadline),
        Mocks.MockWorkspace,
        Mocks.MockTemplate,
      )
    },
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
}

export const WithoutUpdateAccess = Template.bind({})
WithoutUpdateAccess.args = {
  ...Started.args,
  canUpdateWorkspace: false,
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
  workspaceErrors: {
    [WorkspaceErrors.BUILD_ERROR]: Mocks.makeMockApiError({
      message: "A workspace build is already active.",
    }),
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

export const GetBuildsError = Template.bind({})
GetBuildsError.args = {
  ...Started.args,
  workspaceErrors: {
    [WorkspaceErrors.GET_BUILDS_ERROR]: Mocks.makeMockApiError({
      message: "There is a problem fetching builds.",
    }),
  },
}

export const GetResourcesError = Template.bind({})
GetResourcesError.args = {
  ...Started.args,
  workspaceErrors: {
    [WorkspaceErrors.GET_RESOURCES_ERROR]: Mocks.makeMockApiError({
      message: "There is a problem fetching workspace resources.",
    }),
  },
}

export const CancellationError = Template.bind({})
CancellationError.args = {
  ...Error.args,
  workspaceErrors: {
    [WorkspaceErrors.CANCELLATION_ERROR]: Mocks.makeMockApiError({
      message: "Job could not be canceled.",
    }),
  },
}
