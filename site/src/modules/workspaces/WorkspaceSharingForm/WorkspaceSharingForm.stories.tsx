import type { Meta, StoryObj } from "@storybook/react-vite";
import type { TemplateACL, WorkspaceACL } from "#/api/typesGenerated";
import {
	MockGroup,
	MockGroup2,
	MockOrganization,
	MockTemplateACL,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { WorkspaceSharingForm } from "./WorkspaceSharingForm";

const mockWorkspaceACL: WorkspaceACL = {
	users: [
		{ ...MockUserOwner, role: "admin" },
		{ ...MockUserMember, role: "use" },
	],
	group: [{ ...MockGroup, role: "use" }],
};

const meta: Meta<typeof WorkspaceSharingForm> = {
	title: "modules/workspaces/WorkspaceSharingForm",
	component: WorkspaceSharingForm,
	args: {
		organizationId: MockOrganization.id,
		canUpdatePermissions: true,
		error: undefined,
		updatingUserId: undefined,
		updatingGroupId: undefined,
		onUpdateUser: () => {},
		onRemoveUser: () => {},
		onUpdateGroup: () => {},
		onRemoveGroup: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceSharingForm>;

export const Empty: Story = {
	args: {
		workspaceACL: { users: [], group: [] },
		templateACL: MockTemplateACL,
	},
};

export const WithMembers: Story = {
	args: {
		workspaceACL: mockWorkspaceACL,
		templateACL: MockTemplateACL,
	},
};

// The template's ACL includes the "everyone" group (org ID), so all members
// have access and no warnings are shown.
export const AllMembersHaveTemplateAccess: Story = {
	args: {
		workspaceACL: mockWorkspaceACL,
		templateACL: MockTemplateACL,
	},
};

// The template is restricted: only specific users/groups in its ACL can
// access it. Members not in the template ACL see a warning icon and a
// summary alert appears above the table.
export const SomeMembersLackTemplateAccess: Story = {
	args: {
		workspaceACL: {
			users: [
				{ ...MockUserOwner, role: "admin" },
				{ ...MockUserMember, role: "use" },
			],
			group: [
				{ ...MockGroup, role: "use" },
				{ ...MockGroup2, role: "use" },
			],
		},
		// Restricted template: no "everyone" group, only MockUserOwner and
		// MockGroup have access. MockUserMember and MockGroup2 lack access.
		templateACL: {
			users: [{ ...MockUserOwner, role: "use" }],
			group: [{ ...MockGroup, role: "use" }],
		} satisfies TemplateACL,
	},
};

// When template ACL is undefined (still loading or the caller lacks
// permission to read it), no warnings are shown.
export const TemplateACLUnavailable: Story = {
	args: {
		workspaceACL: mockWorkspaceACL,
		templateACL: undefined,
	},
};

// Compact variant used in the share popover.
export const CompactWithWarnings: Story = {
	args: {
		isCompact: true,
		workspaceACL: {
			users: [
				{ ...MockUserOwner, role: "admin" },
				{ ...MockUserMember, role: "use" },
			],
			group: [{ ...MockGroup, role: "use" }],
		},
		templateACL: {
			users: [{ ...MockUserOwner, role: "use" }],
			group: [],
		} satisfies TemplateACL,
	},
};
