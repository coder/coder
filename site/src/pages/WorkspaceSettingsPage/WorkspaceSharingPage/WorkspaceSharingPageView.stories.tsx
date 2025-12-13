import {
	MockGroup,
	MockGroup2,
	MockUserMember,
	MockUserOwner,
	MockWorkspace,
	mockApiError,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type {
	WorkspaceACL,
	WorkspaceGroup,
	WorkspaceUser,
} from "api/typesGenerated";
import { WorkspaceSharingPageView } from "./WorkspaceSharingPageView";

const mockWorkspaceUser: WorkspaceUser = {
	id: MockUserMember.id,
	username: MockUserMember.username,
	name: MockUserMember.name,
	avatar_url: MockUserMember.avatar_url,
	role: "use",
};

const mockWorkspaceUserAdmin: WorkspaceUser = {
	id: MockUserOwner.id,
	username: MockUserOwner.username,
	name: MockUserOwner.name,
	avatar_url: MockUserOwner.avatar_url,
	role: "admin",
};

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
	users: [mockWorkspaceUser, mockWorkspaceUserAdmin],
	group: [],
};

const aclWithGroups: WorkspaceACL = {
	users: [],
	group: [mockWorkspaceGroup, mockWorkspaceGroupAdmin],
};

const aclWithUsersAndGroups: WorkspaceACL = {
	users: [mockWorkspaceUser, mockWorkspaceUserAdmin],
	group: [mockWorkspaceGroup, mockWorkspaceGroupAdmin],
};

const meta: Meta<typeof WorkspaceSharingPageView> = {
	title: "pages/WorkspaceSharingPageView",
	component: WorkspaceSharingPageView,
	args: {
		workspace: MockWorkspace,
		workspaceACL: emptyACL,
		canUpdatePermissions: true,
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
		canUpdatePermissions: false,
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
		updatingUserId: mockWorkspaceUser.id,
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
