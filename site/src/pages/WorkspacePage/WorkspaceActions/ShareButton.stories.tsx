import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { getWorkspaceSharingSettingsKey } from "#/api/queries/organizations";
import { workspaceACLKey } from "#/api/queries/workspaces";
import type { WorkspaceACL } from "#/api/typesGenerated";
import {
	MockGroup,
	MockUserMember,
	MockUserOwner,
	MockWorkspace,
} from "#/testHelpers/entities";
import { ShareButton } from "./ShareButton";

const mockWorkspaceACL: WorkspaceACL = {
	users: [
		{ ...MockUserMember, role: "use" },
		{ ...MockUserOwner, role: "admin" },
		{
			...MockUserMember,
			id: "long-username-user",
			username: "a-very-long-username-that-must-wrap",
			role: "use",
		},
	],
	group: [{ ...MockGroup, role: "use" }],
};

const meta: Meta<typeof ShareButton> = {
	title: "pages/WorkspacePage/ShareButton",
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
			{ key: workspaceACLKey(MockWorkspace.id), data: mockWorkspaceACL },
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: /share/i }),
		);
	},
};

export default meta;
type Story = StoryObj<typeof ShareButton>;

export const Open: Story = {};

export const OpenNarrowViewport: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
};
