import {
	MockConnectedSSHConnectionLog,
	MockWebConnectionLog,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
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

export const WebTerminal: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			type: "reconnecting_pty",
		},
	},
};
