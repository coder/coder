import type { Meta, StoryObj } from "@storybook/react-vite";
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

export const Default: Story = {};
