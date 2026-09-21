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
			type_display_name: "Port Forwarding",
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
			type_display_name: "VS Code",
		},
	},
};

// An IDE the agent named itself, stored as-is.
export const Cursor: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			type: "cursor",
			type_display_name: "Cursor",
		},
	},
};

// An IDE Coder does not recognize presents as its own identifier.
export const UnregisteredIDE: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			type: "some_new_ide",
			type_display_name: "some_new_ide",
		},
	},
};

export const JetBrains: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			type: "jetbrains",
			type_display_name: "JetBrains",
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

export const WebTerminal: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			type: "reconnecting_pty",
			type_display_name: "Web Terminal",
		},
	},
};
