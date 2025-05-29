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
		workspace: MockWorkspace,
		portSharingControlsEnabled: true,
		host: "coder.com",
	},
};

export default meta;
type Story = StoryObj<typeof PortForwardPopoverView>;

export const WithPorts: Story = {
	args: {
		listeningPorts: MockListeningPortsResponse.ports,
		sharedPorts: MockSharedPortsResponse.shares,
	},
};

export const WithManyPorts: Story = {
	args: {
		listeningPorts: Array.from({ length: 20 }).map((_, i) => ({
			process_name: `port-${i}`,
			network: "",
			port: 3000 + i,
		})),
		sharedPorts: MockSharedPortsResponse.shares,
	},
};

export const Empty: Story = {
	args: {
		listeningPorts: [],
		sharedPorts: [],
	},
};

export const AGPLPortSharing: Story = {
	args: {
		listeningPorts: MockListeningPortsResponse.ports,
		portSharingControlsEnabled: false,
		sharedPorts: MockSharedPortsResponse.shares,
	},
};

export const EnterprisePortSharingControlsOwner: Story = {
	args: {
		listeningPorts: MockListeningPortsResponse.ports,
		sharedPorts: [],
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
		sharedPorts: MockSharedPortsResponse.shares.filter((share) => {
			return share.share_level === "authenticated";
		}),
	},
};
