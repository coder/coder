import {
	MockOutdatedStoppedWorkspaceRequireActiveVersion,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockTemplateVersionParameter3,
	MockWorkspace,
	MockWorkspaceBuildParameter1,
	MockWorkspaceBuildParameter2,
	MockWorkspaceBuildParameter3,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { WorkspaceParametersPageView } from "./WorkspaceParametersPage";

const meta: Meta<typeof WorkspaceParametersPageView> = {
	title: "pages/WorkspaceSettingsPage/WorkspaceParametersPageView",
	component: WorkspaceParametersPageView,
	args: {
		submitError: undefined,
		isSubmitting: false,
		workspace: MockWorkspace,
		canChangeVersions: true,
		onCancel: action("onCancel"),
		buildParameters: [
			MockWorkspaceBuildParameter1,
			MockWorkspaceBuildParameter2,
			MockWorkspaceBuildParameter3,
		],
		templateVersionParameters: [
			MockTemplateVersionParameter1,
			MockTemplateVersionParameter2,
			{
				...MockTemplateVersionParameter3,
				mutable: false,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceParametersPageView>;

const Example: Story = {};

export const Empty: Story = {
	args: {
		buildParameters: [],
		templateVersionParameters: [],
	},
};

export const RequireActiveVersionNoChangeVersion: Story = {
	args: {
		workspace: MockOutdatedStoppedWorkspaceRequireActiveVersion,
		canChangeVersions: false,
		buildParameters: [
			MockWorkspaceBuildParameter1,
			MockWorkspaceBuildParameter2,
			MockWorkspaceBuildParameter3,
		],
		templateVersionParameters: [
			MockTemplateVersionParameter1,
			MockTemplateVersionParameter2,
			{
				...MockTemplateVersionParameter3,
				mutable: false,
			},
		],
	},
};

export const RequireActiveVersionCanChangeVersion: Story = {
	args: {
		workspace: MockOutdatedStoppedWorkspaceRequireActiveVersion,
		canChangeVersions: true,
		buildParameters: [
			MockWorkspaceBuildParameter1,
			MockWorkspaceBuildParameter2,
			MockWorkspaceBuildParameter3,
		],
		templateVersionParameters: [
			MockTemplateVersionParameter1,
			MockTemplateVersionParameter2,
			{
				...MockTemplateVersionParameter3,
				mutable: false,
			},
		],
	},
};

export { Example as WorkspaceParametersPage };
