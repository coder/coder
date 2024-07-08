import type { Meta, StoryObj } from "@storybook/react";
import {
  MockListeningPortsResponse,
  MockSharedPortsResponse,
  MockTemplate,
  MockWorkspace,
  MockWorkspaceAgent,
} from "testHelpers/entities";
import { PortForwardPopoverView } from "./PortForwardButton";

const meta: Meta<typeof PortForwardPopoverView> = {
  title: "modules/resources/PortForwardPopoverView",
  component: PortForwardPopoverView,
  decorators: [
    (Story) => (
      <div
        css={(theme) => ({
          width: 404,
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
    template: MockTemplate,
    workspaceID: MockWorkspace.id,
    portSharingControlsEnabled: true,
  },
};

export default meta;
type Story = StoryObj<typeof PortForwardPopoverView>;

export const WithPorts: Story = {
  args: {
    listeningPorts: MockListeningPortsResponse.ports,
  },
  parameters: {
    queries: [
      {
        key: ["sharedPorts", MockWorkspace.id],
        data: MockSharedPortsResponse,
      },
    ],
  },
};

export const WithManyPorts: Story = {
  args: {
    listeningPorts: Array.from({ length: 20 }).map((_, i) => ({
      process_name: `port-${i}`,
      network: "",
      port: 3000 + i,
    })),
  },
  parameters: {
    queries: [
      {
        key: ["sharedPorts", MockWorkspace.id],
        data: MockSharedPortsResponse,
      },
    ],
  },
};

export const Empty: Story = {
  args: {
    listeningPorts: [],
  },
  parameters: {
    queries: [
      {
        key: ["sharedPorts", MockWorkspace.id],
        data: { shares: [] },
      },
    ],
  },
};

export const AGPLPortSharing: Story = {
  args: {
    listeningPorts: MockListeningPortsResponse.ports,
    portSharingControlsEnabled: false,
  },
  parameters: {
    queries: [
      {
        key: ["sharedPorts", MockWorkspace.id],
        data: MockSharedPortsResponse,
      },
    ],
  },
};

export const EnterprisePortSharingControlsOwner: Story = {
  args: {
    listeningPorts: MockListeningPortsResponse.ports,
    template: {
      ...MockTemplate,
      max_port_share_level: "owner",
    },
  },
};

export const EnterprisePortSharingControlsAuthenticated: Story = {
  args: {
    listeningPorts: MockListeningPortsResponse.ports,
    template: {
      ...MockTemplate,
      max_port_share_level: "authenticated",
    },
  },
  parameters: {
    queries: [
      {
        key: ["sharedPorts", MockWorkspace.id],
        data: {
          shares: MockSharedPortsResponse.shares.filter((share) => {
            return share.share_level === "authenticated";
          }),
        },
      },
    ],
  },
};
