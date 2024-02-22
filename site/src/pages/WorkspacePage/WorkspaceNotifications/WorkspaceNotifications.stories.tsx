import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import {
  MockOutdatedWorkspace,
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersionWithMarkdownMessage,
  MockWorkspace,
} from "testHelpers/entities";
import { getWorkspaceResolveAutostartQueryKey } from "api/queries/workspaceQuota";
import { withDashboardProvider } from "testHelpers/storybook";
import { WorkspaceNotifications } from "./WorkspaceNotifications";

const defaultPermissions = {
  readWorkspace: true,
  updateTemplate: true,
  updateWorkspace: true,
  viewDeploymentValues: true,
};

const meta: Meta<typeof WorkspaceNotifications> = {
  title: "pages/WorkspacePage/WorkspaceNotifications",
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
  },

  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByTestId("info-notifications"));
      await waitFor(() =>
        expect(
          screen.getByText(MockTemplateVersion.message),
        ).toBeInTheDocument(),
      );
    });
  },
};

export const OutdatedWithMarkdownMessage: Story = {
  args: {
    workspace: MockOutdatedWorkspace,
    latestVersion: MockTemplateVersionWithMarkdownMessage,
  },

  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByTestId("info-notifications"));
      await waitFor(() =>
        expect(screen.getByText(/an update is available/i)).toBeInTheDocument(),
      );
    });
  },
};

export const RequiresManualUpdate: Story = {
  args: {
    workspace: {
      ...MockOutdatedWorkspace,
      automatic_updates: "always",
      autostart_schedule: "daily",
    },
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

  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByTestId("warning-notifications"));
      await waitFor(() =>
        expect(
          screen.getByText(/unable to automatically update/i),
        ).toBeInTheDocument(),
      );
    });
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
  },

  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByTestId("warning-notifications"));
      await waitFor(() =>
        expect(screen.getByText(/workspace is unhealthy/i)).toBeInTheDocument(),
      );
    });
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

  play: Unhealthy.play,
};

const DormantWorkspace = {
  ...MockWorkspace,
  dormant_at: new Date("2020-01-01T00:00:00Z").toISOString(),
};

export const Dormant: Story = {
  args: {
    workspace: DormantWorkspace,
  },

  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByTestId("warning-notifications"));
      await waitFor(() =>
        expect(screen.getByText(/workspace is dormant/i)).toBeInTheDocument(),
      );
    });
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

  play: Dormant.play,
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
  },

  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(await screen.findByTestId("info-notifications"));
      await waitFor(() =>
        expect(screen.getByText(/build is pending/i)).toBeInTheDocument(),
      );
    });
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
  },

  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByTestId("warning-notifications"));
      await waitFor(() =>
        expect(screen.getByText(/deprecated template/i)).toBeInTheDocument(),
      );
    });
  },
};
