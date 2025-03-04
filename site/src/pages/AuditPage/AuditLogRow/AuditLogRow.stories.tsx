import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import {
	MockAuditLog,
	MockAuditLog2,
	MockAuditLogGitSSH,
	MockAuditLogRequestPasswordReset,
	MockAuditLogWithDeletedResource,
	MockAuditLogWithWorkspaceBuild,
	MockUser,
} from "testHelpers/entities";
import { AuditLogRow } from "./AuditLogRow";

const meta: Meta<typeof AuditLogRow> = {
	title: "pages/AuditPage/AuditLogRow",
	component: AuditLogRow,
	decorators: [
		(Story) => (
			<TableContainer>
				<Table>
					<TableHead>
						<TableRow>
							<TableCell style={{ paddingLeft: 32 }}>Logs</TableCell>
						</TableRow>
					</TableHead>
					<TableBody>
						<Story />
					</TableBody>
				</Table>
			</TableContainer>
		),
	],
};

export default meta;
type Story = StoryObj<typeof AuditLogRow>;

export const NoDiff: Story = {
	args: {
		auditLog: {
			...MockAuditLog,
			diff: {},
		},
	},
};

export const WithDiff: Story = {
	parameters: { chromatic },
	args: {
		auditLog: MockAuditLog2,
		defaultIsDiffOpen: true,
	},
};

export const WithLongDiffRow: Story = {
	parameters: { chromatic },
	args: {
		auditLog: {
			...MockAuditLog2,
			diff: {
				...MockAuditLog2.diff,
				icon: {
					old: "https://www.google.com/url?sa=i&url=https%3A%2F%2Fwww.docker.com%2Fcompany%2Fnewsroom%2Fmedia-resources%2F&psig=AOvVaw3hLg_lm0tzXPBt74XZD2GC&ust=1666892413988000&source=images&cd=vfe&ved=0CAwQjRxqFwoTCPDsiKa4_voCFQAAAAAdAAAAABAD",
					new: "https://www.google.com/url?sa=i&url=https%3A%2F%2Fwww.kindpng.com%2Fimgv%2FhRowRxi_docker-icon-png-transparent-png%2F&psig=AOvVaw3hLg_lm0tzXPBt74XZD2GC&ust=1666892413988000&source=images&cd=vfe&ved=0CAwQjRxqFwoTCPDsiKa4_voCFQAAAAAdAAAAABAI",
					secret: false,
				},
			},
		},
		defaultIsDiffOpen: true,
	},
};

export const WithStoppedWorkspaceBuild: Story = {
	args: {
		auditLog: {
			...MockAuditLogWithWorkspaceBuild,
			action: "stop",
		},
	},
};

export const WithStartedWorkspaceBuild: Story = {
	args: {
		auditLog: {
			...MockAuditLogWithWorkspaceBuild,
			action: "start",
		},
	},
};

export const WithDeletedWorkspaceBuild: Story = {
	args: {
		auditLog: {
			...MockAuditLogWithWorkspaceBuild,
			action: "delete",
			is_deleted: true,
		},
	},
};

export const DeletedResource: Story = {
	args: {
		auditLog: MockAuditLogWithDeletedResource,
	},
};

export const SecretDiffValue: Story = {
	args: {
		auditLog: MockAuditLogGitSSH,
	},
};

export const WithOrganization: Story = {
	args: {
		auditLog: MockAuditLog,
		showOrgDetails: true,
	},
};

export const WithDateDiffValue: Story = {
	args: {
		auditLog: MockAuditLogRequestPasswordReset,
	},
};

export const NoUserAgent: Story = {
	args: {
		auditLog: {
			id: "8073939e-2f18-41f6-9cec-c1e61293b0d5",
			request_id: "79d1df16-b387-4d47-8f47-dc2b919c78b9",
			time: "2024-07-15T19:30:16.327247Z",
			organization_id: "703f72a1-76f6-4f89-9de6-8a3989693fe5",
			ip: "",
			user_agent: "",
			resource_type: "workspace_build",
			resource_id: "605e8bda-2d1e-43c3-beec-97ebedc1b88c",
			resource_target: "",
			resource_icon: "",
			action: "delete",
			diff: {},
			status_code: 500,
			additional_fields: {
				build_number: "35",
				build_reason: "autodelete",
				workspace_id: "649742dc-1b4a-43d8-8539-2fbc11b1bbac",
				workspace_name: "yeee",
				workspace_owner: "",
			},
			description: "{user} deleted workspace {target}",
			resource_link: "/@jon/yeee/builds/35",
			is_deleted: false,
			user: MockUser,
		},
	},
};

export const WithConnectionType: Story = {
	args: {
		showOrgDetails: true,
		auditLog: {
			id: "725ea2f2-faae-4bdd-a821-c2384a67d89c",
			request_id: "a486c1cb-6acb-41c9-9bce-1f4f24a2e8ff",
			time: "2025-02-24T10:20:08.054072Z",
			ip: "fd7a:115c:a1e0:4fa5:9ccd:27e4:5d72:c66a",
			user_agent: "",
			resource_type: "workspace_agent",
			resource_id: "813311fb-bad3-4a92-98cd-09ee57e73d6e",
			resource_target: "main",
			resource_icon: "",
			action: "disconnect",
			diff: {},
			status_code: 255,
			additional_fields: {
				reason: "process exited with error status: -1",
				build_number: "1",
				build_reason: "initiator",
				workspace_id: "6a7cfb32-d208-47bb-91d0-ec54b69912b6",
				workspace_name: "test2",
				connection_type: "SSH",
				workspace_owner: "admin",
			},
			description: "{user} disconnected workspace agent {target}",
			resource_link: "",
			is_deleted: false,
			organization_id: "0e6fa63f-b625-4a6f-ab5b-a8217f8c80b3",
			organization: {
				id: "0e6fa63f-b625-4a6f-ab5b-a8217f8c80b3",
				name: "coder",
				display_name: "Coder",
				icon: "",
			},
			user: null,
		},
	},
};
