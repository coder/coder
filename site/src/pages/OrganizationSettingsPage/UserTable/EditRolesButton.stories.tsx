import {
	MockOwnerRole,
	MockSiteRoles,
	MockUserAdminRole,
	MockWorkspaceCreationBanRole,
} from "testHelpers/entities";
import { withDesktopViewport } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { EditRolesButton } from "./EditRolesButton";

const meta: Meta<typeof EditRolesButton> = {
	title: "pages/UsersPage/EditRolesButton",
	component: EditRolesButton,
	args: {
		selectedRoleNames: new Set([MockUserAdminRole.name, MockOwnerRole.name]),
		roles: MockSiteRoles,
	},
	decorators: [withDesktopViewport],
};

export default meta;
type Story = StoryObj<typeof EditRolesButton>;

export const Closed: Story = {};

export const Open: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
	},
};

export const Loading: Story = {
	args: {
		isLoading: true,
		userLoginType: "password",
		oidcRoleSync: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
	},
};

export const CannotSetRoles: Story = {
	args: {
		userLoginType: "oidc",
		oidcRoleSync: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(canvas.getByLabelText("More info"));
	},
};

export const AdvancedOpen: Story = {
	args: {
		selectedRoleNames: new Set([MockWorkspaceCreationBanRole.name]),
		roles: MockSiteRoles,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
	},
};
