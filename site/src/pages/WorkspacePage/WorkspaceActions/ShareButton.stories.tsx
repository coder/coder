import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { getWorkspaceSharingSettingsKey } from "#/api/queries/organizations";
import { workspaceACLKey } from "#/api/queries/workspaces";
import type {
	WorkspaceACL,
	WorkspaceSharingSettings,
} from "#/api/typesGenerated";
import {
	MockGroup,
	MockGroup2,
	MockGroup3,
	MockGroupWithoutMembers,
	MockWorkspace,
	MockWorkspaceUser,
	MockWorkspaceUserAdmin,
} from "#/testHelpers/entities";
import { ShareButton } from "./ShareButton";

const mockSharingSettings: WorkspaceSharingSettings = {
	sharing_disabled: false,
	sharing_globally_disabled: false,
	shareable_workspace_owners: "everyone",
};

const mockOverflowingACL: WorkspaceACL = {
	users: [MockWorkspaceUserAdmin, MockWorkspaceUser],
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

export const Loading: Story = {
	parameters: {
		queries: [
			{
				key: getWorkspaceSharingSettingsKey(MockWorkspace.organization_id),
				data: mockSharingSettings,
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceACL").mockReturnValue(new Promise(() => {}));
	},
	play: ({ canvasElement }) => openSharing(canvasElement),
};

export const ReadOnly: Story = {
	args: {
		canShareWorkspace: false,
	},
	parameters: {
		queries: [
			{
				key: getWorkspaceSharingSettingsKey(MockWorkspace.organization_id),
				data: mockSharingSettings,
			},
			{ key: workspaceACLKey(MockWorkspace.id), data: mockOverflowingACL },
		],
	},
	play: async ({ canvasElement }) => {
		await openSharing(canvasElement);
		const list = screen.getByRole("region", {
			name: "Shared workspace members and groups",
		});
		for (let i = 0; i < 10 && document.activeElement !== list; i++) {
			await userEvent.tab();
		}
	},
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
	play: async ({ canvasElement }) => {
		await openSharing(canvasElement);
		// Scroll so the screenshot shows rows passing under the sticky header.
		screen.getAllByRole("row").at(-1)?.scrollIntoView({ block: "end" });
	},
};
