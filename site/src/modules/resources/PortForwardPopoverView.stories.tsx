import { PortForwardPopoverView } from "./PortForwardButton";
import type { Meta, StoryObj } from "@storybook/react";
import {
  MockListeningPortsResponse,
  MockSharedPortsResponse,
  MockWorkspaceAgent,
} from "testHelpers/entities";

const meta: Meta<typeof PortForwardPopoverView> = {
  title: "modules/resources/PortForwardPopoverView",
  component: PortForwardPopoverView,
  decorators: [
    (Story) => (
      <div
        css={(theme) => ({
          width: 304,
          border: `1px solid ${theme.palette.divider}`,
          borderRadius: 8,
          backgroundColor: theme.palette.background.paper,
        })}
      >
        <Story />
      </div>
    ),
  ],
  args: {
    agent: MockWorkspaceAgent,
  },
};

export default meta;
type Story = StoryObj<typeof PortForwardPopoverView>;

export const WithPorts: Story = {
  args: {
    listeningPorts: MockListeningPortsResponse.ports,
    storybook: {
      sharedPortsQueryData: MockSharedPortsResponse
    },
  },
};

export const Empty: Story = {
  args: {
    listeningPorts: [],

  },
};
