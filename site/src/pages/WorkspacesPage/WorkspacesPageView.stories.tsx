import { ComponentMeta, Story } from "@storybook/react"
import { DEFAULT_RECORDS_PER_PAGE } from "components/PaginationWidget/utils"
import dayjs from "dayjs"
import uniqueId from "lodash/uniqueId"
import {
  Workspace,
  WorkspaceStatus,
  WorkspaceStatuses,
} from "../../api/typesGenerated"
import { MockWorkspace } from "../../testHelpers/entities"
import { workspaceFilterQuery } from "../../util/filters"
import {
  WorkspacesPageView,
  WorkspacesPageViewProps,
} from "./WorkspacesPageView"

const createWorkspace = (
  status: WorkspaceStatus,
  outdated = false,
  lastUsedAt = "0001-01-01",
): Workspace => {
  return {
    ...MockWorkspace,
    id: uniqueId("workspace"),
    outdated,
    latest_build: {
      ...MockWorkspace.latest_build,
      status,
    },
    last_used_at: lastUsedAt,
  }
}

// This is type restricted to prevent future statuses from slipping
// through the cracks unchecked!
const workspaces = WorkspaceStatuses.map((status) => createWorkspace(status))

// Additional Workspaces depending on time
const additionalWorkspaces: Record<string, Workspace> = {
  today: createWorkspace(
    "running",
    true,
    dayjs().subtract(3, "hour").toString(),
  ),
  old: createWorkspace("running", true, dayjs().subtract(1, "week").toString()),
  veryOld: createWorkspace(
    "running",
    true,
    dayjs().subtract(1, "month").subtract(4, "day").toString(),
  ),
}

const allWorkspaces = [
  ...Object.values(workspaces),
  ...Object.values(additionalWorkspaces),
]

export default {
  title: "pages/WorkspacesPageView",
  component: WorkspacesPageView,
  argTypes: {
    limit: {
      defaultValue: DEFAULT_RECORDS_PER_PAGE,
    },
  },
} as ComponentMeta<typeof WorkspacesPageView>

const Template: Story<WorkspacesPageViewProps> = (args) => (
  <WorkspacesPageView {...args} />
)

export const AllStates = Template.bind({})
AllStates.args = {
  workspaces: allWorkspaces,
  count: allWorkspaces.length,
}

export const OwnerHasNoWorkspaces = Template.bind({})
OwnerHasNoWorkspaces.args = {
  workspaces: [],
  filter: workspaceFilterQuery.me,
  count: 0,
}

export const NoSearchResults = Template.bind({})
NoSearchResults.args = {
  workspaces: [],
  filter: "searchtearmwithnoresults",
  count: 0,
}
