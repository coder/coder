import { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within, screen } from "@storybook/test";
import {
  MockTemplate,
  MockTemplateVersion,
  MockUser,
  MockWorkspace,
} from "testHelpers/entities";
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
    template: MockTemplate,
    latestVersion: MockTemplateVersion,
  },
  parameters: {
    layout: "fullscreen",
    features: ["advanced_template_scheduling"],
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceTopbar>;

export const Example: Story = {};

export const Outdated: Story = {
  args: {
    workspace: {
      ...baseWorkspace,
      outdated: true,
    },
  },
};

export const Ready: Story = {
  args: {
    workspace: {
      ...baseWorkspace,
      last_used_at: new Date().toISOString(),
      latest_build: {
        ...baseWorkspace.latest_build,
        updated_at: new Date().toISOString(),
      },
    },
  },
};

export const Connected: Story = {
  args: {
    workspace: {
      ...baseWorkspace,
      last_used_at: new Date().toISOString(),
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

export const WithExceededDeadline: Story = {
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

const in30Minutes = new Date();
in30Minutes.setMinutes(in30Minutes.getMinutes() + 30);
export const WithApproachingDeadline: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      latest_build: {
        ...MockWorkspace.latest_build,
        deadline: in30Minutes.toISOString(),
      },
    },
  },
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(canvas.getByTestId("schedule-controls-autostop"));
      await waitFor(() =>
        expect(screen.getByRole("tooltip")).toHaveTextContent(
          /this workspace has enabled autostop/,
        ),
      );
    });
  },
};

const in8Hours = new Date();
in8Hours.setHours(in8Hours.getHours() + 8);
export const WithFarAwayDeadline: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      latest_build: {
        ...MockWorkspace.latest_build,
        deadline: in8Hours.toISOString(),
      },
    },
  },
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(canvas.getByTestId("schedule-controls-autostop"));
      await waitFor(() =>
        expect(screen.getByRole("tooltip")).toHaveTextContent(
          /this workspace has enabled autostop/,
        ),
      );
    });
  },
};

export const WithFarAwayDeadlineRequiredByTemplate: Story = {
  args: {
    workspace: {
      ...MockWorkspace,
      latest_build: {
        ...MockWorkspace.latest_build,
        deadline: in8Hours.toISOString(),
      },
    },
    template: {
      ...MockTemplate,
      allow_user_autostop: false,
    },
  },
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(canvas.getByTestId("schedule-controls-autostop"));
      await waitFor(() =>
        expect(screen.getByRole("tooltip")).toHaveTextContent(
          /template has an autostop requirement/,
        ),
      );
    });
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
