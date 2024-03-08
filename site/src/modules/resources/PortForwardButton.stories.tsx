import type { Meta, StoryObj } from "@storybook/react";
import {
  MockListeningPortsResponse,
  MockSharedPortsResponse,
  MockWorkspace,
  MockWorkspaceAgent,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { PortForwardButton } from "./PortForwardButton";

const meta: Meta<typeof PortForwardButton> = {
  title: "modules/resources/PortForwardButton",
  component: PortForwardButton,
  decorators: [withDashboardProvider],
  args: {
    agent: MockWorkspaceAgent,
  },
};

export default meta;
type Story = StoryObj<typeof PortForwardButton>;

export const Example: Story = {
  parameters: {
    queries: [
      {
        key: ["portForward", MockWorkspaceAgent.id],
        data: MockListeningPortsResponse,
      },
      {
        key: ["sharedPorts", MockWorkspace.id],
        data: MockSharedPortsResponse,
      },
    ],
  },
};

export const Loading: Story = {};
