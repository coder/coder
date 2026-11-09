import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { getWorkspaceSharingSettingsKey } from "#/api/queries/organizations";
import type { User, WorkspaceRole } from "#/api/typesGenerated";
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

const toWorkspaceUser = (user: User, role: WorkspaceRole) => ({
	id: user.id,
	username: user.username,
	name: user.name,
	avatar_url: user.avatar_url,
	role,
});

const meta: Meta<typeof ShareButton> = {
	title: "pages/WorkspacePage/WorkspaceActions/ShareButton",
	component: ShareButton,
	args: {
		workspace: MockWorkspace,
		canShareWorkspace: true,
	},
	parameters: {
		queries: [
			{
				key: getWorkspaceSharingSettingsKey(MockWorkspace.organization_id),
				data: { sharing_disabled: false },
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ShareButton>;

export const OverflowingMembers: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaceACL").mockResolvedValue({
			users: [
				toWorkspaceUser(MockUserOwner, "admin"),
				toWorkspaceUser(MockUserMember, "use"),
			],
			group: [
				{ ...MockGroup, role: "use" },
				{ ...MockGroup2, role: "admin" },
				{ ...MockGroup3, role: "use" },
				{ ...MockGroupWithoutMembers, role: "use" },
			],
		});
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Share" }),
		);
		await within(document.body).findByRole("columnheader", {
			name: "Member",
		});
	},
};
