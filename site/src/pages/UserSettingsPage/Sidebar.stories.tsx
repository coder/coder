import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { Sidebar } from "./Sidebar";

const meta: Meta<typeof Sidebar> = {
	title: "pages/UserSettingsPage/Sidebar",
	component: Sidebar,
	parameters: {
		user: MockUserOwner,
		permissions: { viewWorkspaces: true },
		features: ["advanced_template_scheduling"],
	},
	decorators: [withAuthProvider, withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof Sidebar>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Schedule");
		await canvas.findByText("SSH Keys");
	},
};

export const WithoutWorkspaceAccess: Story = {
	parameters: {
		permissions: { viewWorkspaces: false },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Account");

		expect(canvas.queryByText("Schedule")).not.toBeInTheDocument();
		expect(canvas.queryByText("SSH Keys")).not.toBeInTheDocument();
	},
};
