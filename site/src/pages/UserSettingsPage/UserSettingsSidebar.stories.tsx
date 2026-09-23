import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { UserSettingsSidebar } from "./UserSettingsSidebar";
import { UserSettingsSidebarHeader } from "./UserSettingsSidebarView";

const STORAGE_KEY = "story-user-settings-sidebar-connected";

const meta: Meta<typeof UserSettingsSidebar> = {
	title: "pages/UserSettingsPage/UserSettingsSidebar",
	component: UserSettingsSidebar,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		(Story) => (
			<CollapsibleSidebar
				label="Your account"
				storageKey={STORAGE_KEY}
				header={<UserSettingsSidebarHeader />}
			>
				<Story />
			</CollapsibleSidebar>
		),
	],
	beforeEach: () => {
		localStorage.setItem(STORAGE_KEY, "expanded");
		return () => localStorage.removeItem(STORAGE_KEY);
	},
	parameters: { user: MockUserOwner },
};

export default meta;
type Story = StoryObj<typeof UserSettingsSidebar>;

// Explicit so the story does not depend on the fixture default.
export const OAuth2ProviderEnabled: Story = {
	parameters: { buildInfo: { oauth2_provider: true } },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("link", { name: "OAuth2 applications" }),
		).toBeVisible();
	},
};

// The OAuth2 item follows the deployment flag, not the build type, so a
// development build with the flag off still hides it.
export const OAuth2ProviderDisabled: Story = {
	parameters: {
		buildInfo: { version: "v2.99.99-devel+abcdef", oauth2_provider: false },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.queryByRole("link", { name: "OAuth2 applications" }),
		).toBeNull();
		expect(
			canvas.getByRole("link", { name: "External authentication" }),
		).toBeVisible();
	},
};
