import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	MockConnectedSSHConnectionLog,
	MockDeniedTunnelConnectionLog,
	MockTunnelConnectionLog,
	MockWebConnectionLog,
} from "#/testHelpers/entities";
import { ConnectionLogDescription } from "./ConnectionLogDescription";

const meta: Meta<typeof ConnectionLogDescription> = {
	title: "pages/ConnectionLogPage/ConnectionLogDescription",
	component: ConnectionLogDescription,
};

export default meta;
type Story = StoryObj<typeof ConnectionLogDescription>;

// Stories that set connection_method also set a contradictory deprecated
// `type`, so the screenshots prove rendering follows connection_method.
export const SSH: Story = {
	args: {
		connectionLog: {
			...MockConnectedSSHConnectionLog,
			type: "port_forwarding",
			connection_method: "ssh",
		},
	},
};

export const App: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
		},
	},
};

export const AppUnauthenticated: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			web_info: {
				...MockWebConnectionLog.web_info!,
				user: null,
			},
		},
	},
};

export const AppAuthenticatedFail: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			web_info: {
				...MockWebConnectionLog.web_info!,
				status_code: 404,
			},
		},
	},
};

export const PortForwardingAuthenticated: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			type: "workspace_app",
			connection_method: "port_forwarding",
			web_info: {
				...MockWebConnectionLog.web_info!,
				slug_or_port: "8080",
			},
		},
	},
};

export const AppUnauthenticatedRedirect: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			web_info: {
				...MockWebConnectionLog.web_info!,
				user: null,
				status_code: 303,
			},
		},
	},
};

export const SSHWithApp: Story = {
	args: {
		connectionLog: {
			...MockConnectedSSHConnectionLog,
			type: "workspace_app",
			connection_method: "ssh",
			app_name: "cursor",
			app_display_name: "Cursor",
		},
	},
};

export const SSHWithUnknownApp: Story = {
	args: {
		connectionLog: {
			...MockConnectedSSHConnectionLog,
			type: "tunnel",
			connection_method: "ssh",
			app_name: "an_unregistered_ide",
			app_display_name: "an_unregistered_ide",
		},
	},
};

export const Tunnel: Story = {
	args: {
		connectionLog: MockTunnelConnectionLog,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/established a tunnel to/)).toBeVisible();
	},
};

// An admin tunneling into another user's workspace, which is the
// primary audit scenario for tunnel events.
export const TunnelOtherUser: Story = {
	args: {
		connectionLog: {
			...MockTunnelConnectionLog,
			workspace_owner_username: "some-other-user",
		},
	},
};

export const TunnelDenied: Story = {
	args: {
		connectionLog: MockDeniedTunnelConnectionLog,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/was denied a tunnel to/)).toBeVisible();
	},
};

export const ReconnectingPTY: Story = {
	args: {
		connectionLog: {
			...MockConnectedSSHConnectionLog,
			type: "ssh",
			connection_method: "reconnecting_pty",
		},
	},
};
