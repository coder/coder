import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { ProvisionerJobStatus, Workspace } from "../../api/typesGenerated"
import { MockWorkspace } from "../../testHelpers/entities"
import { WorkspacesPageView, WorkspacesPageViewProps } from "./WorkspacesPageView"

export default {
  title: "pages/WorkspacesPageView",
  component: WorkspacesPageView,
} as ComponentMeta<typeof WorkspacesPageView>

const Template: Story<WorkspacesPageViewProps> = (args) => <WorkspacesPageView {...args} />

const createWorkspaceWithStatus = (status: ProvisionerJobStatus, transition = "start"): Workspace => {
  return {
    ...MockWorkspace,
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
  workspaces: [
    ...Object.values(workspaces),
    createWorkspaceWithStatus("running", "stop"),
    createWorkspaceWithStatus("succeeded", "stop"),
    createWorkspaceWithStatus("running", "delete"),
  ],
}

export const Empty = Template.bind({})
Empty.args = {}
