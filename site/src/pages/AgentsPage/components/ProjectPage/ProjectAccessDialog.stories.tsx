import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent } from "storybook/test";
import { groupsByOrganization } from "#/api/queries/groups";
import { organizationMembers } from "#/api/queries/organizations";
import type { ChatProjectACL } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectACL,
	MockGroup3,
	MockUserMember,
	mockApiError,
} from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import { ProjectAccessDialog } from "./ProjectAccessDialog";
import { buildAccessEntries } from "./projectAccess";

const mockACL: ChatProjectACL = {
	...MockChatProjectACL,
	users: [
		{
			...MockUserMember,
			name: "Maximiliana Bartholomea Featherstonehaugh",
			role: "admin",
		},
		{
			...MockUserMember,
			id: "sharee-2",
			username: "grace",
			name: "",
			role: "use",
		},
	],
};

const meta: Meta<typeof ProjectAccessDialog> = {
	title: "pages/AgentsPage/ProjectPage/ProjectAccessDialog",
	component: ProjectAccessDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		entries: buildAccessEntries(mockACL),
		error: undefined,
		isRetrying: false,
		onRetry: fn(),
		onCloseAutoFocus: fn(),
		canManage: false,
		organizationId: MockChatProject.organization_id,
		currentUserId: MockChatProjectACL.owner.id,
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
		entries: buildAccessEntries({ ...mockACL, users: [], groups: [] }),
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
		entries: buildAccessEntries({ ...mockACL, users: [], groups: [] }),
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

export const ManageRowMenu: Story = {
	args: { canManage: true },
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Open menu for grace" }),
		);
	},
};

export const ManagePendingChange: Story = {
	args: { canManage: true, pendingPrincipalId: "sharee-2" },
};

const orgId = MockChatProject.organization_id;

/** Back-End is picked in the add row and its add is saving. */
export const ManagePendingAdd: Story = {
	args: { canManage: true, pendingPrincipalId: MockGroup3.id },
	parameters: {
		queries: [
			{ key: groupsByOrganization(orgId).queryKey, data: [MockGroup3] },
			{
				key: organizationMembers(orgId, { limit: 0 }).queryKey,
				data: { members: [], count: 0 },
			},
		],
	},
	// The role comes first: picking the pending principal disables its select.
	play: async () => {
		await userEvent.click(
			await screen.findByRole("combobox", { name: "Role" }),
		);
		await userEvent.click(await screen.findByRole("option", { name: /Admin/ }));
		await userEvent.click(
			await screen.findByRole("button", { name: /^User or group/ }),
		);
		await userEvent.click(await screen.findByRole("option", { name: /Back/ }));
	},
};

export const ManageChangeError: Story = {
	args: {
		canManage: true,
		mutationError: mockApiError({
			message: "Failed to update chat project ACL.",
		}),
	},
};

/** A 403 skips the generic detail about viewing content. */
export const ManageForbiddenError: Story = {
	args: {
		canManage: true,
		mutationError: {
			...mockApiError({
				message: "Forbidden.",
				detail: "You don't have permission to view this content.",
			}),
			status: 403,
		},
	},
};

/** Rows stack on phones, and long names truncate. */
export const ManageOnPhone: Story = {
	args: { canManage: true },
	parameters: { pixel: { matrix: pixelWithPhone } },
};
