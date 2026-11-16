import type { Meta, StoryObj } from "@storybook/react-vite";
import { getWorkspaceSharingSettingsKey } from "#/api/queries/organizations";
import type { WorkspaceACL, WorkspaceGroup } from "#/api/typesGenerated";
import {
	MockGroup,
	MockGroup2,
	MockWorkspace,
	MockWorkspaceUser,
	MockWorkspaceUserAdmin,
	mockApiError,
} from "#/testHelpers/entities";
import { WorkspaceSharingPageView } from "./WorkspaceSharingPageView";

const mockWorkspaceGroup: WorkspaceGroup = {
	...MockGroup,
	role: "use",
};

const mockWorkspaceGroupAdmin: WorkspaceGroup = {
	...MockGroup2,
	role: "admin",
};

const emptyACL: WorkspaceACL = {
	users: [],
	group: [],
};

const aclWithUsers: WorkspaceACL = {
	users: [MockWorkspaceUser, MockWorkspaceUserAdmin],
	group: [],
};

const aclWithGroups: WorkspaceACL = {
	users: [],
	group: [mockWorkspaceGroup, mockWorkspaceGroupAdmin],
};

const aclWithUsersAndGroups: WorkspaceACL = {
	users: [MockWorkspaceUser, MockWorkspaceUserAdmin],
	group: [mockWorkspaceGroup, mockWorkspaceGroupAdmin],
};

const meta: Meta<typeof WorkspaceSharingPageView> = {
	title: "pages/WorkspaceSharingPageView",
	component: WorkspaceSharingPageView,
	parameters: {
		queries: [
			{
				key: getWorkspaceSharingSettingsKey(MockWorkspace.organization_id),
				data: { sharing_disabled: false },
			},
		],
	},
	args: {
		workspace: MockWorkspace,
		workspaceACL: emptyACL,
		canShareWorkspace: true,
		error: undefined,
		onAddUser: () => Promise.resolve(),
		isAddingUser: false,
		onUpdateUser: () => Promise.resolve(),
		updatingUserId: undefined,
		onRemoveUser: () => Promise.resolve(),
		onAddGroup: () => Promise.resolve(),
		isAddingGroup: false,
		onUpdateGroup: () => Promise.resolve(),
		updatingGroupId: undefined,
		onRemoveGroup: () => Promise.resolve(),
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceSharingPageView>;

export const Empty: Story = {};

export const Loading: Story = {
	args: {
		workspaceACL: undefined,
	},
};

export const WithUsers: Story = {
	args: {
		workspaceACL: aclWithUsers,
	},
};

export const WithGroups: Story = {
	args: {
		workspaceACL: aclWithGroups,
	},
};

export const WithUsersAndGroups: Story = {
	args: {
		workspaceACL: aclWithUsersAndGroups,
	},
};

export const ReadOnly: Story = {
	args: {
		workspaceACL: aclWithUsersAndGroups,
		canShareWorkspace: false,
	},
};

export const AddingMember: Story = {
	args: {
		workspaceACL: aclWithUsers,
		isAddingUser: true,
	},
};

export const UpdatingUser: Story = {
	args: {
		workspaceACL: aclWithUsers,
		updatingUserId: MockWorkspaceUser.id,
	},
};

export const UpdatingGroup: Story = {
	args: {
		workspaceACL: aclWithGroups,
		updatingGroupId: mockWorkspaceGroup.id,
	},
};

export const WithError: Story = {
	args: {
		workspaceACL: aclWithUsersAndGroups,
		error: mockApiError({
			message: "Failed to update workspace sharing settings",
			detail: "You do not have permission to modify this workspace.",
		}),
	},
};
