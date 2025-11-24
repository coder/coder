import {
	MockTemplateVersion,
	MockTemplateVersionWithMarkdownMessage,
	MockWorkspace,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { templateVersionsQueryKey } from "api/queries/templates";
import { ChangeWorkspaceVersionDialog } from "./ChangeWorkspaceVersionDialog";

const noMessage = {
	...MockTemplateVersion,
	name: "no-message",
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

export const CurrentVersion: Story = {};

export const NoMessage: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				template_version_id: noMessage.id,
			},
		},
	},
};

export const TextMessage: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				template_version_id: MockTemplateVersion.id,
			},
		},
	},
};

export const MarkdownMessage: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				template_version_id: MockTemplateVersionWithMarkdownMessage.id,
			},
		},
	},
};
