import type { Meta, StoryObj } from "@storybook/react";
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
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
    isDefaultOpen: true,
    sshPrefix: "coder.",
  },
};
