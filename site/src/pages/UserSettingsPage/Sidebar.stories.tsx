import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { MockUserOwner } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { Sidebar } from "./Sidebar";

const meta: Meta<typeof Sidebar> = {
	title: "pages/UserSettingsPage/Sidebar",
	component: Sidebar,
	decorators: [withDashboardProvider],
	parameters: {
		features: ["advanced_template_scheduling"],
		experiments: ["oauth2"],
	},
	args: {
		user: MockUserOwner,
	},
};

export default meta;
type Story = StoryObj<typeof Sidebar>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const navItems = canvas.getAllByRole("link").map((item) =>
			(item.textContent ?? "")
				.replace(/\(.*?\)/g, "")
				.replace(/beta/i, "")
				.replace(/\s+/g, " ")
				.trim(),
		);

		expect(navItems.length).toBeGreaterThan(0);
		expect(navItems).toEqual(navItems.toSorted((a, b) => a.localeCompare(b)));
	},
};
