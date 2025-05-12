import type { Meta, StoryObj } from "@storybook/react";
import {
	MockTemplate,
	MockTemplateVersion,
	MockTemplateVersionWithMarkdownMessage,
	MockWorkspace,
} from "testHelpers/entities";
import { ChangeWorkspaceVersionDialog } from "./ChangeWorkspaceVersionDialog";
import { templateVersionsQueryKey } from "api/queries/templates";

const noMessage = {
	...MockTemplateVersion,
	id: "no-message",
	message: "",
};

const meta: Meta<typeof ChangeWorkspaceVersionDialog> = {
	title: "modules/workspaces/ChangeWorkspaceVersionDialog",
	component: ChangeWorkspaceVersionDialog,
	args: {
		open: true,
		workspace: MockWorkspace,
	},
	parameters: {
		queries: [
			{
				key: templateVersionsQueryKey(MockWorkspace.template_id),
				data: [
					MockTemplateVersion,
					MockTemplateVersionWithMarkdownMessage,
					noMessage,
				],
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ChangeWorkspaceVersionDialog>;

export const NoVersionSelected: Story = {};

export const NoMessage: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			template_active_version_id: noMessage.id,
		},
	},
};

export const ShortMessage: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			template_active_version_id: MockTemplateVersion.id,
		},
	},
};

export const LongMessage: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			template_active_version_id: MockTemplateVersionWithMarkdownMessage.id,
		},
	},
};
