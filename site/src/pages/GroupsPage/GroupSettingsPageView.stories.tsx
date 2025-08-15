import { MockGroup } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import GroupSettingsPageView from "./GroupSettingsPageView";

const meta: Meta<typeof GroupSettingsPageView> = {
	title: "pages/OrganizationGroupsPage/GroupSettingsPageView",
	component: GroupSettingsPageView,
	args: {
		onCancel: action("onCancel"),
		group: MockGroup,
		isLoading: false,
	},
};

export default meta;
type Story = StoryObj<typeof GroupSettingsPageView>;

const Example: Story = {};

export { Example as GroupSettingsPageView };
