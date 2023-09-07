import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { SSHButton } from "./SSHButton";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof SSHButton> = {
  title: "components/SSHButton",
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
    defaultIsOpen: true,
    sshPrefix: "coder.",
  },
};
