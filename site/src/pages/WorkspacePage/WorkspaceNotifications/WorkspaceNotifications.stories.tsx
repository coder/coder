import {
  MockOutdatedWorkspace,
  MockTemplate,
  MockTemplateVersion,
  MockWorkspace,
} from "testHelpers/entities";
import { WorkspaceNotifications } from "./WorkspaceNotifications";
import type { Meta, StoryObj } from "@storybook/react";
import { withDashboardProvider } from "testHelpers/storybook";
import { getWorkspaceResolveAutostartQueryKey } from "api/queries/workspaceQuota";

const defaultPermissions = {
  readWorkspace: true,
  updateTemplate: true,
  updateWorkspace: true,
  viewDeploymentValues: true,
};

const meta: Meta<typeof WorkspaceNotifications> = {
  title: "components/WorkspaceNotifications",
  component: WorkspaceNotifications,
  args: {
    latestVersion: MockTemplateVersion,
    template: MockTemplate,
    workspace: MockWorkspace,
    permissions: defaultPermissions,
  },
  decorators: [withDashboardProvider],
  parameters: {
    queries: [
      {
        key: getWorkspaceResolveAutostartQueryKey(MockOutdatedWorkspace.id),
        data: {
          parameter_mismatch: false,
        },
      },
    ],
    features: ["advanced_template_scheduling"],
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceNotifications>;

export const Outdated: Story = {
  args: {
    workspace: MockOutdatedWorkspace,
    defaultOpen: "info",
  },
};

export const RequiresManualUpdate: Story = {
  args: {
    workspace: {
      ...MockOutdatedWorkspace,
      automatic_updates: "always",
      autostart_schedule: "daily",
    },
    defaultOpen: "warning",
  },
  parameters: {
    queries: [
      {
        key: getWorkspaceResolveAutostartQueryKey(MockOutdatedWorkspace.id),
        data: {
          parameter_mismatch: true,
        },
      },
    ],
  },
};

export const Unhealthy: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      health: {
        ...MockWorkspace.health,
        healthy: false,
      },
      latest_build: {
        ...MockWorkspace.latest_build,
        status: "running",
      },
    },
    defaultOpen: "warning",
  },
};

export const UnhealthyWithoutUpdatePermission: Story = {
  args: {
    ...Unhealthy.args,
    permissions: {
      ...defaultPermissions,
      updateWorkspace: false,
    },
  },
};

const DormantWorkspace = {
  ...MockWorkspace,
  dormant_at: new Date("2020-01-01T00:00:00Z").toISOString(),
};

export const Dormant: Story = {
  args: {
    defaultOpen: "warning",
    workspace: DormantWorkspace,
  },
};

export const DormantWithDeletingDate: Story = {
  args: {
    ...Dormant.args,
    workspace: {
      ...DormantWorkspace,
      deleting_at: new Date("2020-10-01T00:00:00Z").toISOString(),
    },
  },
};

export const PendingInQueue: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      latest_build: {
        ...MockWorkspace.latest_build,
        status: "pending",
        job: {
          ...MockWorkspace.latest_build.job,
          queue_size: 10,
          queue_position: 3,
        },
      },
    },
    defaultOpen: "info",
  },
};

export const TemplateDeprecated: Story = {
  args: {
    template: {
      ...MockTemplate,
      deprecated: true,
      deprecation_message:
        "Template deprecated due to reasons. [Learn more](#)",
    },
    defaultOpen: "warning",
  },
};
