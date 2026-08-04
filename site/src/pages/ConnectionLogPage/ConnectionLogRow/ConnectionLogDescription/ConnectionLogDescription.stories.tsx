import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
	MockCoderdTunnelConnectionLog,
	MockConnectedSSHConnectionLog,
	MockDeniedTunnelConnectionLog,
	MockTunnelConnectionLog,
	MockWebConnectionLog,
	MockWorkspaceProxyTunnelConnectionLog,
} from "#/testHelpers/entities";
import { ConnectionLogDescription } from "./ConnectionLogDescription";

const meta: Meta<typeof ConnectionLogDescription> = {
	title: "pages/ConnectionLogPage/ConnectionLogDescription",
	component: ConnectionLogDescription,
};

export default meta;
type Story = StoryObj<typeof ConnectionLogDescription>;

export const SSH: Story = {
	args: {
		connectionLog: MockConnectedSSHConnectionLog,
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
			type: "port_forwarding",
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

export const VSCode: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			type: "vscode",
		},
	},
};

export const JetBrains: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			type: "jetbrains",
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

export const TunnelCoderdSystem: Story = {
	args: {
		connectionLog: MockCoderdTunnelConnectionLog,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Coder system established a tunnel to/),
		).toBeVisible();
	},
};

export const TunnelWorkspaceProxy: Story = {
	args: {
		connectionLog: MockWorkspaceProxyTunnelConnectionLog,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Workspace proxy established a tunnel to/),
		).toBeVisible();
	},
};

export const TunnelUnknownSystem: Story = {
	args: {
		connectionLog: {
			...MockCoderdTunnelConnectionLog,
			web_info: {
				user: null,
				user_agent: "internal-component",
				slug_or_port: "",
				status_code: 101,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/System actor established a tunnel to/),
		).toBeVisible();
	},
};

export const WebTerminal: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			type: "reconnecting_pty",
		},
	},
};
