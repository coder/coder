import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { VSCodeDesktopButton } from "./VSCodeDesktopButton";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof VSCodeDesktopButton> = {
  title: "components/VSCodeDesktopButton",
  component: VSCodeDesktopButton,
};

export default meta;
type Story = StoryObj<typeof VSCodeDesktopButton>;

export const Default: Story = {
  args: {
    userName: MockWorkspace.owner_name,
    workspaceName: MockWorkspace.name,
    agentName: MockWorkspaceAgent.name,
    displayApps: [
      "vscode",
      "port_forwarding_helper",
      "ssh_helper",
      "vscode_insiders",
      "web_terminal",
    ],
  },
};
