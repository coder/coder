import { ComponentMeta, Story } from "@storybook/react"
import dayjs from "dayjs"
import { spawn } from "xstate"
import { ProvisionerJobStatus, WorkspaceTransition } from "../../api/typesGenerated"
import { MockWorkspace } from "../../testHelpers/entities"
import { workspaceFilterQuery } from "../../util/filters"
import {
  workspaceItemMachine,
  WorkspaceItemMachineRef,
} from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView, WorkspacesPageViewProps } from "./WorkspacesPageView"

const createWorkspaceItemRef = (
  status: ProvisionerJobStatus,
  transition: WorkspaceTransition = "start",
  outdated = false,
  lastUsedAt = "0001-01-01",
): WorkspaceItemMachineRef => {
  return spawn(
    workspaceItemMachine.withContext({
      data: {
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
        last_used_at: lastUsedAt,
      },
    }),
  )
}

// This is type restricted to prevent future statuses from slipping
// through the cracks unchecked!
const workspaces: { [key in ProvisionerJobStatus]: WorkspaceItemMachineRef } = {
  canceled: createWorkspaceItemRef("canceled"),
  canceling: createWorkspaceItemRef("canceling"),
  failed: createWorkspaceItemRef("failed"),
  pending: createWorkspaceItemRef("pending"),
  running: createWorkspaceItemRef("running"),
  succeeded: createWorkspaceItemRef("succeeded"),
}

const additionalWorkspaces: Record<string, WorkspaceItemMachineRef> = {
  runningAndStop: createWorkspaceItemRef("running", "stop"),
  succeededAndStop: createWorkspaceItemRef("succeeded", "stop"),
  runningAndDelete: createWorkspaceItemRef("running", "delete"),
  outdated: createWorkspaceItemRef("running", "delete", true),
  active: createWorkspaceItemRef("running", undefined, true, dayjs().toString()),
  today: createWorkspaceItemRef("running", undefined, true, dayjs().subtract(3, "hour").toString()),
  old: createWorkspaceItemRef("running", undefined, true, dayjs().subtract(1, "week").toString()),
  veryOld: createWorkspaceItemRef(
    "running",
    undefined,
    true,
    dayjs().subtract(1, "month").subtract(4, "day").toString(),
  ),
}

export default {
  title: "pages/WorkspacesPageView",
  component: WorkspacesPageView,
  argTypes: {
    workspaceRefs: {
      options: [...Object.keys(workspaces), ...Object.keys(additionalWorkspaces)],
      mapping: { ...workspaces, ...additionalWorkspaces },
    },
  },
} as ComponentMeta<typeof WorkspacesPageView>

const Template: Story<WorkspacesPageViewProps> = (args) => <WorkspacesPageView {...args} />

export const AllStates = Template.bind({})
AllStates.args = {
  workspaceRefs: [...Object.values(workspaces), ...Object.values(additionalWorkspaces)],
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
