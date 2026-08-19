import type { Meta, StoryObj } from "@storybook/react-vite";
import { Table, TableBody } from "#/components/Table/Table";
import {
	MockConnectedSSHConnectionLog,
	MockDisconnectedSSHConnectionLog,
	MockFileOperationConnectionLog,
	MockFileTransferConnectionLog,
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

export const BlockedFileTransfer: Story = {
	args: {
		connectionLog: MockFileTransferConnectionLog,
	},
};

export const CompletedFileTransfer: Story = {
	args: {
		connectionLog: {
			...MockFileTransferConnectionLog,
			ssh_info: {
				...MockFileTransferConnectionLog.ssh_info!,
				disconnect_reason: "",
				exit_code: 0,
			},
		},
	},
};

export const FileOperationDownload: Story = {
	args: {
		connectionLog: MockFileOperationConnectionLog,
	},
};

export const FileOperationUpload: Story = {
	args: {
		connectionLog: {
			...MockFileOperationConnectionLog,
			file_transfer_info: {
				...MockFileOperationConnectionLog.file_transfer_info!,
				action: "upload",
				path: "/home/coder/upload.tar.gz",
			},
		},
	},
};

export const FileOperationRename: Story = {
	args: {
		connectionLog: {
			...MockFileOperationConnectionLog,
			file_transfer_info: {
				...MockFileOperationConnectionLog.file_transfer_info!,
				action: "rename",
				path: "/home/coder/old-name.txt",
				target: "/home/coder/new-name.txt",
			},
		},
	},
};

export const FileOperationSCPUpload: Story = {
	args: {
		connectionLog: {
			...MockFileOperationConnectionLog,
			file_transfer_info: {
				...MockFileOperationConnectionLog.file_transfer_info!,
				protocol: "scp",
				action: "upload",
				path: "/home/coder/project",
			},
		},
	},
};

export const FileOperationLongPath: Story = {
	args: {
		connectionLog: {
			...MockFileOperationConnectionLog,
			file_transfer_info: {
				...MockFileOperationConnectionLog.file_transfer_info!,
				path: "/home/coder/some/deeply/nested/directory/structure/with/a/very/long/path/to/an/important/file/that/should/not/break/the/layout.json",
			},
		},
	},
};
