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
			id: "a26a8a29-9453-455c-8fdc-ce69d4e2c07b",
			request_id: "0d649adb-016d-4d8f-8448-875bbdf30c74",
			time: "2025-02-21T14:18:39.198013Z",
			ip: "fd7a:115c:a1e0:4955:8019:f5d3:e126:b422",
			user_agent: "",
			resource_type: "workspace_agent",
			resource_id: "a146c7e1-514b-4534-8f98-3f097fb83b11",
			resource_target: "main",
			resource_icon: "",
			action: "disconnect",
			diff: {},
			status_code: 0,
			additional_fields: {
				build_number: "5",
				build_reason: "initiator",
				workspace_id: "d28295ae-a2dc-4aa0-af3c-79a2d4c44c55",
				workspace_name: "test",
				connection_type: "VS Code",
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
