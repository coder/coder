import type { Meta, StoryObj } from "@storybook/react";
import {
	MockAuditLog,
	MockAuditLogRequestPasswordReset,
	MockAuditLogSuccessfulLogin,
	MockAuditLogUnsuccessfulLoginKnownUser,
	MockAuditLogWithWorkspaceBuild,
	MockWorkspaceCreateAuditLogForDifferentOwner,
} from "testHelpers/entities";
import { AuditLogDescription } from "./AuditLogDescription";

const meta: Meta<typeof AuditLogDescription> = {
	title: "pages/AuditPage/AuditLogDescription",
	component: AuditLogDescription,
};

export default meta;
type Story = StoryObj<typeof AuditLogDescription>;

export const WorkspaceCreate: Story = {
	args: {
		auditLog: MockAuditLog,
	},
};

export const WorkspaceBuildStop: Story = {
	args: {
		auditLog: MockAuditLogWithWorkspaceBuild,
	},
};

export const WorkspaceBuildDuplicatedWord: Story = {
	args: {
		auditLog: {
			...MockAuditLogWithWorkspaceBuild,
			additional_fields: {
				workspace_name: "workspace",
			},
		},
	},
};

export const CreateWorkspaceWithDiffOwner: Story = {
	args: {
		auditLog: MockWorkspaceCreateAuditLogForDifferentOwner,
	},
};

export const SuccessLogin: Story = {
	args: {
		auditLog: MockAuditLogSuccessfulLogin,
	},
};

export const UnsuccessfulLoginForUnknownUser: Story = {
	args: {
		auditLog: MockAuditLogUnsuccessfulLoginKnownUser,
	},
};

export const RequestPasswordReset: Story = {
	args: {
		auditLog: MockAuditLogRequestPasswordReset,
	},
};

export const CreateUser: Story = {
	args: {
		auditLog: {
			...MockAuditLog,
			resource_type: "user",
			resource_target: "colin",
			description: "{user} created user {target}",
		},
	},
};

export const SCIMCreateUser: Story = {
	args: {
		auditLog: {
			...MockAuditLog,
			resource_type: "user",
			resource_target: "colin",
			description: "{user} created user {target}",
			additional_fields: {
				automatic_actor: "coder",
				automatic_subsystem: "scim",
			},
		},
	},
};

export const SCIMUpdateUser: Story = {
	args: {
		auditLog: {
			...MockAuditLog,
			action: "write",
			resource_type: "user",
			resource_target: "colin",
			description: "{user} updated user {target}",
			additional_fields: {
				automatic_actor: "coder",
				automatic_subsystem: "scim",
			},
		},
	},
};

export const UnauthenticatedUser: Story = {
	args: {
		auditLog: {
			...MockAuditLog,
			user: null,
		},
	},
};
