import { ComponentMeta, Story } from "@storybook/react"
import { spawn } from "xstate"
import { ProvisionerJobStatus, Workspace, WorkspaceTransition } from "../../api/typesGenerated"
import { MockWorkspace } from "../../testHelpers/entities"
import { workspaceFilterQuery } from "../../util/workspace"
import { workspaceItemMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView, WorkspacesPageViewProps } from "./WorkspacesPageView"

export default {
  title: "pages/WorkspacesPageView",
  component: WorkspacesPageView,
} as ComponentMeta<typeof WorkspacesPageView>

const Template: Story<WorkspacesPageViewProps> = (args) => <WorkspacesPageView {...args} />

const createWorkspaceWithStatus = (
  status: ProvisionerJobStatus,
  transition: WorkspaceTransition = "start",
  outdated = false,
): Workspace => {
  return {
    ...MockWorkspace,
    outdated,
    latest_build: {
      ...MockWorkspace.latest_build,
      transition,
      job: {
        ...MockWorkspace.latest_build.job,
        status: status,
      },
    },
  }
}

// This is type restricted to prevent future statuses from slipping
// through the cracks unchecked!
const workspaces: { [key in ProvisionerJobStatus]: Workspace } = {
  canceled: createWorkspaceWithStatus("canceled"),
  canceling: createWorkspaceWithStatus("canceling"),
  failed: createWorkspaceWithStatus("failed"),
  pending: createWorkspaceWithStatus("pending"),
  running: createWorkspaceWithStatus("running"),
  succeeded: createWorkspaceWithStatus("succeeded"),
}

export const AllStates = Template.bind({})
AllStates.args = {
  workspaceRefs: [
    ...Object.values(workspaces),
    createWorkspaceWithStatus("running", "stop"),
    createWorkspaceWithStatus("succeeded", "stop"),
    createWorkspaceWithStatus("running", "delete"),
  ].map((data) => spawn(workspaceItemMachine.withContext({ data }))),
}

export const OwnerHasNoWorkspaces = Template.bind({})
OwnerHasNoWorkspaces.args = {
  workspaceRefs: [],
  filter: workspaceFilterQuery.me,
}

export const NoResults = Template.bind({})
NoResults.args = {
  workspaceRefs: [],
  filter: "searchtearmwithnoresults",
}
