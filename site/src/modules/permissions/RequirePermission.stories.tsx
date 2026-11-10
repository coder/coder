import type { Meta, StoryObj } from "@storybook/react-vite";
import { RequirePermission } from "./RequirePermission";

const meta: Meta<typeof RequirePermission> = {
	title: "modules/permissions/RequirePermission",
	component: RequirePermission,
	args: {
		isFeatureVisible: false,
		children: <p>Protected page content</p>,
	},
};

export default meta;
type Story = StoryObj<typeof RequirePermission>;

export const Denied: Story = {};

export const Allowed: Story = {
	args: {
		isFeatureVisible: true,
	},
};
