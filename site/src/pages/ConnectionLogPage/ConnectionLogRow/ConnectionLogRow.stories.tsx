import {
	MockConnectedSSHConnectionLog,
	MockDisconnectedSSHConnectionLog,
	MockWebConnectionLog,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Table, TableBody } from "components/Table/Table";
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

export const Web: Story = {
	args: {
		connectionLog: MockWebConnectionLog,
	},
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
};

export const DisconnectedSSH: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
		},
	},
};

export const DisconnectedSSHError: Story = {
	args: {
		connectionLog: {
			...MockDisconnectedSSHConnectionLog,
			ssh_info: {
				...MockDisconnectedSSHConnectionLog.ssh_info!,
				exit_code: 130, // 128 + SIGINT
			},
		},
	},
};
