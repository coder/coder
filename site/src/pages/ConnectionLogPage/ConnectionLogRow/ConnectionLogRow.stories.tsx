import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { Table, TableBody } from "#/components/Table/Table";
import {
	MockConnectedSSHConnectionLog,
	MockDisconnectedSSHConnectionLog,
	MockTunnelConnectionLog,
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

export const Tunnel: Story = {
	args: {
		connectionLog: MockTunnelConnectionLog,
	},
};

export const OngoingSSH: Story = {
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

export const NoConnectRecorded: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
			connect_time: MockDisconnectedSSHConnectionLog.ssh_info.disconnect_time,
			ssh_info: {
				...MockDisconnectedSSHConnectionLog.ssh_info,
			},
		},
	},
};

export const DisconnectBeforeConnect: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
			ssh_info: {
				...MockDisconnectedSSHConnectionLog.ssh_info,
				disconnect_time: "2022-05-19T16:40:57.122Z",
			},
		},
	},
};
