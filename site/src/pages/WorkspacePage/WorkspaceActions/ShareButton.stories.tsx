import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { getWorkspaceSharingSettingsKey } from "#/api/queries/organizations";
import { workspaceACLKey } from "#/api/queries/workspaces";
import type {
	WorkspaceACL,
	WorkspaceSharingSettings,
	WorkspaceUser,
} from "#/api/typesGenerated";
import {
	MockGroup,
	MockGroup2,
	MockGroup3,
	MockGroupWithoutMembers,
	MockUserMember,
	MockUserOwner,
	MockWorkspace,
} from "#/testHelpers/entities";
import { ShareButton } from "./ShareButton";

const mockSharingSettings: WorkspaceSharingSettings = {
	sharing_disabled: false,
	sharing_globally_disabled: false,
	shareable_workspace_owners: "everyone",
};

const mockWorkspaceUserAdmin: WorkspaceUser = {
	id: MockUserOwner.id,
	username: MockUserOwner.username,
	name: MockUserOwner.name,
	avatar_url: MockUserOwner.avatar_url,
	role: "admin",
};

const mockWorkspaceUser: WorkspaceUser = {
	id: MockUserMember.id,
	username: MockUserMember.username,
	name: MockUserMember.name,
	avatar_url: MockUserMember.avatar_url,
	role: "use",
};

const mockOverflowingACL: WorkspaceACL = {
	users: [mockWorkspaceUserAdmin, mockWorkspaceUser],
	group: [
		{ ...MockGroup, role: "use" },
		{ ...MockGroup2, role: "admin" },
		{ ...MockGroup3, role: "use" },
		{ ...MockGroupWithoutMembers, role: "use" },
	],
};

const openSharing = async (canvasElement: HTMLElement) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "Share" }),
	);
	await screen.findByRole("columnheader", { name: "Member" });
};

const meta: Meta<typeof ShareButton> = {
	title: "pages/WorkspacePage/WorkspaceActions/ShareButton",
	component: ShareButton,
	args: {
		workspace: MockWorkspace,
		canShareWorkspace: true,
	},
};

export default meta;
type Story = StoryObj<typeof ShareButton>;

export const EmptyACL: Story = {
	parameters: {
		queries: [
			{
				key: getWorkspaceSharingSettingsKey(MockWorkspace.organization_id),
				data: mockSharingSettings,
			},
			{
				key: workspaceACLKey(MockWorkspace.id),
				data: { users: [], group: [] },
			},
		],
	},
	play: ({ canvasElement }) => openSharing(canvasElement),
};

export const OverflowingMembers: Story = {
	parameters: {
		queries: [
			{
				key: getWorkspaceSharingSettingsKey(MockWorkspace.organization_id),
				data: mockSharingSettings,
			},
			{ key: workspaceACLKey(MockWorkspace.id), data: mockOverflowingACL },
		],
	},
	play: ({ canvasElement }) => openSharing(canvasElement),
};
