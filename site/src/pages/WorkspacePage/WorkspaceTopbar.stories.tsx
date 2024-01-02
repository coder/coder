import { Meta, StoryObj } from "@storybook/react";
import { MockUser, MockWorkspace } from "testHelpers/entities";
import { WorkspaceTopbar } from "./WorkspaceTopbar";
import { withDashboardProvider } from "testHelpers/storybook";
import { addDays } from "date-fns";
import { getWorkspaceQuotaQueryKey } from "api/queries/workspaceQuota";

// We want a workspace without a deadline to not pollute the screenshot
const baseWorkspace = {
  ...MockWorkspace,
  latest_build: {
    ...MockWorkspace.latest_build,
    deadline: undefined,
  },
};

const meta: Meta<typeof WorkspaceTopbar> = {
  title: "pages/WorkspacePage/WorkspaceTopbar",
  component: WorkspaceTopbar,
  decorators: [withDashboardProvider],
  args: {
    workspace: baseWorkspace,
  },
  parameters: {
    layout: "fullscreen",
    features: ["advanced_template_scheduling"],
    experiments: ["workspace_actions"],
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceTopbar>;

export const Example: Story = {};

export const Outdated: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      outdated: true,
    },
  },
};

export const Dormant: Story = {
  args: {
    workspace: {
      ...baseWorkspace,
      deleting_at: addDays(new Date(), 7).toISOString(),
      latest_build: {
        ...baseWorkspace.latest_build,
        status: "failed",
      },
    },
  },
};

export const WithDeadline: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      latest_build: {
        ...MockWorkspace.latest_build,
        deadline: MockWorkspace.latest_build.deadline,
      },
    },
  },
};

export const WithQuota: Story = {
  parameters: {
    queries: [
      {
        key: getWorkspaceQuotaQueryKey(MockUser.username),
        data: {
          credits_consumed: 2,
          budget: 40,
        },
      },
    ],
  },
};
