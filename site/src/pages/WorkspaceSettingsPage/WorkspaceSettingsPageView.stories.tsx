import { MockWorkspace } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { WorkspaceSettingsPageView } from "./WorkspaceSettingsPageView";

const meta: Meta<typeof WorkspaceSettingsPageView> = {
	title: "pages/WorkspaceSettingsPage/WorkspaceSettingsPageView",
	component: WorkspaceSettingsPageView,
	args: {
		error: undefined,
		workspace: MockWorkspace,
		onCancel: action("onCancel"),
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceSettingsPageView>;

export const Example: Story = {};

export const RenamesDisabled: Story = {
	args: {
		workspace: { ...MockWorkspace, allow_renames: false },
	},
};
