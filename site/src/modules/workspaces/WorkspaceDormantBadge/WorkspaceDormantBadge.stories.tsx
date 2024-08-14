import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { MockDormantWorkspace } from "testHelpers/entities";
import { WorkspaceDormantBadge } from "./WorkspaceDormantBadge";

const meta: Meta<typeof WorkspaceDormantBadge> = {
  title: "modules/workspaces/WorkspaceDormantBadge",
  component: WorkspaceDormantBadge,
  args: {
    workspace: MockDormantWorkspace,
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceDormantBadge>;

export const Default: Story = {
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("Open tooltip", async () => {
      await userEvent.hover(canvas.getByRole("status"));
    });
  },
};

export const DeletingAt: Story = {
  args: {
    workspace: {
      ...MockDormantWorkspace,
      deleting_at: "2024-03-12T14:17:12.196Z",
    },
  },
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("Open tooltip", async () => {
      await userEvent.hover(canvas.getByRole("status"));
    });
  },
};
