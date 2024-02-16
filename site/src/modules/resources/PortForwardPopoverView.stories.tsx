import { PortForwardPopoverView } from "./PortForwardButton";
import type { Meta, StoryObj } from "@storybook/react";
import {
  MockListeningPortsResponse,
  MockSharedPortsResponse,
  MockTemplate,
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
    template: MockTemplate,
    portSharingExperimentEnabled: true,
    portSharingControlsEnabled: true,
  },
};

export default meta;
type Story = StoryObj<typeof PortForwardPopoverView>;

export const WithPorts: Story = {
  args: {
    listeningPorts: MockListeningPortsResponse.ports,
    storybook: {
      sharedPortsQueryData: MockSharedPortsResponse,
    },
  },
};

export const Empty: Story = {
  args: {
    listeningPorts: [],
    storybook: {
      sharedPortsQueryData: { shares: [] },
    },
  },
};

export const NoPortSharingExperiment: Story = {
  args: {
    listeningPorts: MockListeningPortsResponse.ports,
    portSharingExperimentEnabled: false,
  },
};

export const AGPLPortSharing: Story = {
  args: {
    listeningPorts: MockListeningPortsResponse.ports,
    storybook: {
      sharedPortsQueryData: MockSharedPortsResponse,
    },
    portSharingControlsEnabled: false,
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
    storybook: {
      sharedPortsQueryData: MockSharedPortsResponse,
    },
    template: {
      ...MockTemplate,
      max_port_share_level: "authenticated",
    },
  },
};
