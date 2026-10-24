import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent } from "storybook/test";
import type { ChatProjectACL } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockGroup,
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { ProjectAccessDialog } from "./ProjectAccessDialog";
import { buildAccessEntries } from "./projectAccess";

const mockACL: ChatProjectACL = {
	users: [
		{ ...MockUserMember, role: "admin" },
		{
			...MockUserMember,
			id: "sharee-2",
			username: "grace",
			name: "",
			role: "use",
		},
	],
	groups: [{ ...MockGroup, role: "use" }],
};

const meta: Meta<typeof ProjectAccessDialog> = {
	title: "pages/AgentsPage/ProjectPage/ProjectAccessDialog",
	component: ProjectAccessDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		entries: buildAccessEntries(MockUserOwner.id, MockUserOwner, mockACL),
		error: undefined,
		isRetrying: false,
		onRetry: fn(),
		onCloseAutoFocus: fn(),
		canManage: false,
		organizationId: MockChatProject.organization_id,
		currentUserId: MockUserOwner.id,
		onAdd: fn(),
		onChangeRole: fn(),
		onRemove: fn(),
		pendingPrincipalId: undefined,
		mutationError: undefined,
	},
};

export default meta;
type Story = StoryObj<typeof ProjectAccessDialog>;

export const Default: Story = {};

export const OwnerOnly: Story = {
	args: {
		entries: buildAccessEntries(MockUserOwner.id, MockUserOwner, {
			users: [],
			groups: [],
		}),
	},
};

export const OwnerLoading: Story = {
	args: {
		entries: buildAccessEntries(MockUserOwner.id, undefined, mockACL),
	},
};

export const OwnerUnknown: Story = {
	args: {
		entries: buildAccessEntries(MockUserOwner.id, null, mockACL),
	},
};

export const Loading: Story = {
	args: { entries: undefined },
};

export const LoadError: Story = {
	args: {
		entries: undefined,
		error: mockApiError({ message: "Failed to get chat project ACL." }),
	},
};

export const Retrying: Story = {
	args: {
		entries: undefined,
		error: mockApiError({ message: "Failed to get chat project ACL." }),
		isRetrying: true,
	},
};

/** A failed refetch keeps the cached access list and shows the error. */
export const RefetchError: Story = {
	args: {
		error: mockApiError({ message: "Failed to get chat project ACL." }),
	},
};

export const Manage: Story = {
	args: { canManage: true },
};

export const ManageOwnerOnly: Story = {
	args: {
		canManage: true,
		entries: buildAccessEntries(MockUserOwner.id, MockUserOwner, {
			users: [],
			groups: [],
		}),
	},
};

/** A project admin cannot change their own role. */
export const ManageAsAdmin: Story = {
	args: { canManage: true, currentUserId: MockUserMember.id },
};

export const ManageRoleOptions: Story = {
	args: { canManage: true },
	play: async () => {
		await userEvent.click(
			await screen.findByRole("combobox", { name: "Role for grace" }),
		);
	},
};

export const ManagePendingChange: Story = {
	args: { canManage: true, pendingPrincipalId: "sharee-2" },
};

export const ManageChangeError: Story = {
	args: {
		canManage: true,
		mutationError: mockApiError({
			message: "Failed to update chat project ACL.",
		}),
	},
};
