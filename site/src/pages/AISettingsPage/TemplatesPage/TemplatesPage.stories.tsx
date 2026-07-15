import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { getTemplatesQueryKey } from "#/api/queries/templates";
import { MockTemplate, MockUserOwner } from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import TemplatesPage from "./TemplatesPage";

const meta = {
	title: "pages/AISettingsPage/TemplatesPage/TemplatesPage",
	component: TemplatesPage,
	decorators: [withAuthProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: {
			editDeploymentConfig: true,
			updateTemplates: true,
		},
		queries: [
			{
				key: getTemplatesQueryKey(),
				data: [MockTemplate],
			},
		],
	},
} satisfies Meta<typeof TemplatesPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const HasBothPermissions: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Test Template")).toBeVisible();
	},
};

export const NoDeploymentConfigPermission: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			updateTemplates: true,
		},
	},
	play: async () => {
		const body = within(document.body);
		expect(
			await body.findByText("You don't have permission to view this page"),
		).toBeInTheDocument();
		expect(body.queryByText("Test Template")).not.toBeInTheDocument();
	},
};

export const NoUpdateTemplatesPermission: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: true,
			updateTemplates: false,
		},
	},
	play: async () => {
		const body = within(document.body);
		expect(
			await body.findByText("You don't have permission to view this page"),
		).toBeInTheDocument();
		expect(body.queryByText("Test Template")).not.toBeInTheDocument();
	},
};
