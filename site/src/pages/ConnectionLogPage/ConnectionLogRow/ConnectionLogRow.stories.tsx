import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { Table, TableBody } from "#/components/Table/Table";
import {
	MockConnectedSSHConnectionLog,
	MockDisconnectedSSHConnectionLog,
	MockWebConnectionLog,
} from "#/testHelpers/entities";
import { ConnectionLogRow } from "./ConnectionLogRow";

const meta: Meta<typeof ConnectionLogRow> = {
	title: "pages/ConnectionLogPage/ConnectionLogRow",
	component: ConnectionLogRow,
	decorators: [
		(Story) => (
			<Table>
				<TableBody>
					<Story />
				</TableBody>
			</Table>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ConnectionLogRow>;

const openConnectionDetails = async () => {
	await userEvent.click(
		screen.getByRole("button", { name: "Connection details" }),
	);
};

export const Web: Story = {
	args: {
		connectionLog: MockWebConnectionLog,
	},
	play: openConnectionDetails,
};

export const WebUnauthenticatedFail: Story = {
	args: {
		connectionLog: {
			...MockWebConnectionLog,
			web_info: {
				status_code: 404,
				user_agent: MockWebConnectionLog.web_info!.user_agent,
				user: null, // Unauthenticated connection attempt
				slug_or_port: MockWebConnectionLog.web_info!.slug_or_port,
			},
		},
	},
};

export const ConnectedSSH: Story = {
	args: {
		connectionLog: MockConnectedSSHConnectionLog,
	},
	play: openConnectionDetails,
};

export const DisconnectedSSH: Story = {
	args: {
		connectionLog: MockDisconnectedSSHConnectionLog,
	},
	play: openConnectionDetails,
};

export const DisconnectedSSHError: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
			ssh_info: {
				...MockDisconnectedSSHConnectionLog.ssh_info,
				exit_code: 130, // 128 + SIGINT
			},
		},
	},
};

export const UnverifiedDuration: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
			ssh_info: {
				...MockDisconnectedSSHConnectionLog.ssh_info,
				duration_status: "unverified",
			},
		},
	},
	play: openConnectionDetails,
};

export const NoConnectRecorded: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
			ssh_info: {
				...MockDisconnectedSSHConnectionLog.ssh_info,
				duration_seconds: undefined,
				duration_status: "missing_connect",
			},
		},
	},
	play: openConnectionDetails,
};

export const DurationUnavailable: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
			ssh_info: {
				...MockDisconnectedSSHConnectionLog.ssh_info,
				duration_seconds: undefined,
				duration_status: "invalid",
			},
		},
	},
	play: openConnectionDetails,
};

export const LegacyClosedInterval: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
			ssh_info: {
				...MockDisconnectedSSHConnectionLog.ssh_info,
				duration_status: undefined,
				duration_seconds: undefined,
				client_session_id: undefined,
			},
		},
	},
	play: openConnectionDetails,
};

export const LegacyNoDisconnect: Story = {
	args: {
		connectionLog: {
			...MockConnectedSSHConnectionLog,
			ssh_info: {
				...MockConnectedSSHConnectionLog.ssh_info,
				duration_status: undefined,
				client_session_id: undefined,
			},
		},
	},
	play: openConnectionDetails,
};

export const LegacyInvalidInterval: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
			ssh_info: {
				...MockDisconnectedSSHConnectionLog.ssh_info,
				duration_status: undefined,
				duration_seconds: undefined,
				client_session_id: undefined,
				disconnect_time: "2022-05-19T16:40:57.122Z",
			},
		},
	},
	play: openConnectionDetails,
};
