import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { withDesktopViewport } from "testHelpers/storybook";
import { SSHButton } from "./SSHButton";

const meta: Meta<typeof SSHButton> = {
  title: "modules/resources/SSHButton",
  component: SSHButton,
};

export default meta;
type Story = StoryObj<typeof SSHButton>;

export const Closed: Story = {
  args: {
    workspaceName: MockWorkspace.name,
    agentName: MockWorkspaceAgent.name,
    sshPrefix: "coder.",
  },
};

export const Opened: Story = {
  args: {
    workspaceName: MockWorkspace.name,
    agentName: MockWorkspaceAgent.name,
    sshPrefix: "coder.",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const button = canvas.getByRole("button");
    await userEvent.click(button);
  },
  decorators: [withDesktopViewport],
};
