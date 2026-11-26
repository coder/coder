import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { MockTemplateACL, MockTemplateACLEmpty } from "#/testHelpers/entities";
import { TemplatePermissionsPageView } from "./TemplatePermissionsPageView";

const meta: Meta<typeof TemplatePermissionsPageView> = {
	title: "pages/TemplateSettingsPage/TemplatePermissionsPageView",
	component: TemplatePermissionsPageView,
};

export default meta;
type Story = StoryObj<typeof TemplatePermissionsPageView>;

export const Empty: Story = {
	args: {
		templateACL: MockTemplateACLEmpty,
		canUpdatePermissions: false,
	},
};

export const WithTemplateACL: Story = {
	args: {
		templateACL: MockTemplateACL,
		canUpdatePermissions: false,
	},
};

export const WithUpdatePermissions: Story = {
	args: {
		templateACL: MockTemplateACL,
		canUpdatePermissions: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", {
				name: "User or group Search for user or group",
			}),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("combobox", { name: "Role" }),
		).toBeInTheDocument();
	},
};
