import type { Meta, StoryObj } from "@storybook/react-vite";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { Sidebar } from "./Sidebar";

const meta: Meta<typeof Sidebar> = {
	title: "pages/UserSettingsPage/Sidebar",
	component: Sidebar,
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof Sidebar>;

export const OAuth2ProviderEnabled: Story = {};

// The OAuth2 item follows the deployment flag, not the build type, so a
// development build with the flag off still hides it.
export const OAuth2ProviderDisabled: Story = {
	parameters: {
		buildInfo: { version: "v2.99.99-devel+abcdef", oauth2_provider: false },
	},
};
