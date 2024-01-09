import { Meta, StoryObj } from "@storybook/react";
import { DEFAULT_RECORDS_PER_PAGE } from "components/PaginationWidget/utils";
import dayjs from "dayjs";
import uniqueId from "lodash/uniqueId";
import {
  Workspace,
  WorkspaceStatus,
  WorkspaceStatuses,
} from "api/typesGenerated";
import {
  MockWorkspace,
  MockAppearanceConfig,
  MockBuildInfo,
  MockEntitlementsWithScheduling,
  MockExperiments,
  mockApiError,
  MockUser,
  MockPendingProvisionerJob,
  MockTemplate,
} from "testHelpers/entities";
import { WorkspacesPageView } from "./WorkspacesPageView";
import { DashboardProviderContext } from "components/Dashboard/DashboardProvider";
import { ComponentProps } from "react";
import {
  MockMenu,
  getDefaultFilterProps,
} from "components/Filter/storyHelpers";

const createWorkspace = (
  status: WorkspaceStatus,
  outdated = false,
  lastUsedAt = "0001-01-01",
  dormantAt?: string,
  deletingAt?: string,
): Workspace => {
  return {
    ...MockWorkspace,
    id: uniqueId("workspace"),
    outdated,
    latest_build: {
      ...MockWorkspace.latest_build,
      status,
      job:
        status === "pending"
          ? MockPendingProvisionerJob
          : MockWorkspace.latest_build.job,
    },
    last_used_at: lastUsedAt,
    dormant_at: dormantAt,
    deleting_at: deletingAt,
  };
};

// This is type restricted to prevent future statuses from slipping
// through the cracks unchecked!
const workspaces = WorkspaceStatuses.map((status) => createWorkspace(status));

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
};

const dormantWorkspaces: Record<string, Workspace> = {
  dormantNoDelete: createWorkspace(
    "stopped",
    false,
    dayjs().subtract(1, "month").toString(),
    dayjs().subtract(1, "month").toString(),
  ),
  dormantAutoDelete: createWorkspace(
    "stopped",
    false,
    dayjs().subtract(1, "month").toString(),
    dayjs().subtract(1, "month").toString(),
    dayjs().add(29, "day").toString(),
  ),
};

const allWorkspaces = [
  ...Object.values(workspaces),
  ...Object.values(additionalWorkspaces),
];

const MockedAppearance = {
  config: MockAppearanceConfig,
  isPreview: false,
  setPreview: () => {},
};

type FilterProps = ComponentProps<typeof WorkspacesPageView>["filterProps"];

const defaultFilterProps = getDefaultFilterProps<FilterProps>({
  query: "owner:me",
  menus: {
    user: MockMenu,
    template: MockMenu,
    status: MockMenu,
  },
  values: {
    owner: MockUser.username,
    template: undefined,
    status: undefined,
  },
});

const mockTemplates = [
  MockTemplate,
  ...[1, 2, 3, 4].map((num) => {
    return {
      ...MockTemplate,
      active_user_count: Math.floor(Math.random() * 10) * num,
      display_name: `Extra Template ${num}`,
      description: "Auto-Generated template",
      icon: num % 2 === 0 ? "" : "/icon/goland.svg",
    };
  }),
];

const meta: Meta<typeof WorkspacesPageView> = {
  title: "pages/WorkspacesPage",
  component: WorkspacesPageView,
  args: {
    limit: DEFAULT_RECORDS_PER_PAGE,
    filterProps: defaultFilterProps,
    checkedWorkspaces: [],
    canCheckWorkspaces: true,
    templates: mockTemplates,
    templatesFetchStatus: "success",
    count: 13,
    page: 1,
  },
  decorators: [
    (Story) => (
      <DashboardProviderContext.Provider
        value={{
          buildInfo: MockBuildInfo,
          entitlements: MockEntitlementsWithScheduling,
          experiments: MockExperiments,
          appearance: MockedAppearance,
        }}
      >
        <Story />
      </DashboardProviderContext.Provider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof WorkspacesPageView>;

export const AllStates: Story = {
  args: {
    workspaces: allWorkspaces,
    count: allWorkspaces.length,
  },
};

const icons = [
  "/icon/code.svg",
  "/icon/aws.svg",
  "/icon/docker-white.svg",
  "/icon/docker.svg",
  "",
  "/icon/doesntexist.svg",
];

export const Icons: Story = {
  args: {
    workspaces: allWorkspaces.map((workspace, i) => ({
      ...workspace,
      template_icon: icons[i % icons.length],
    })),
    count: allWorkspaces.length,
  },
};

export const OwnerHasNoWorkspaces: Story = {
  args: {
    workspaces: [],
    count: 0,
    canCreateTemplate: true,
  },
};

export const OwnerHasNoWorkspacesAndNoTemplates: Story = {
  args: {
    workspaces: [],
    templates: [],
    count: 0,
    canCreateTemplate: true,
  },
};

export const UserHasNoWorkspaces: Story = {
  args: {
    workspaces: [],
    count: 0,
    canCreateTemplate: false,
  },
};

export const UserHasNoWorkspacesAndNoTemplates: Story = {
  args: {
    workspaces: [],
    templates: [],
    count: 0,
    canCreateTemplate: false,
  },
};

export const NoSearchResults: Story = {
  args: {
    workspaces: [],
    filterProps: {
      ...defaultFilterProps,
      filter: {
        ...defaultFilterProps.filter,
        query: "searchwithnoresults",
        used: true,
      },
    },
    count: 0,
  },
};

export const UnhealthyWorkspace: Story = {
  args: {
    workspaces: [
      {
        ...createWorkspace("running"),
        health: {
          healthy: false,
          failing_agents: [],
        },
      },
    ],
  },
};

export const DormantWorkspaces: Story = {
  args: {
    workspaces: Object.values(dormantWorkspaces),
    count: Object.values(dormantWorkspaces).length,
  },
};

export const Error: Story = {
  args: {
    error: mockApiError({ message: "Something went wrong" }),
  },
};
