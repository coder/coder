import type { Meta, StoryObj } from "@storybook/react";
import { MockTemplate, MockTemplateVersion } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { TemplatePageHeader } from "./TemplatePageHeader";

const meta: Meta<typeof TemplatePageHeader> = {
	title: "pages/TemplatePage/TemplatePageHeader",
	component: TemplatePageHeader,
	decorators: [withDashboardProvider],
	args: {
		template: MockTemplate,
		activeVersion: MockTemplateVersion,
		permissions: {
			canUpdateTemplate: true,
		},
		workspacePermissions: {
			createWorkspaceForUser: true,
		},
	},
};

export default meta;
type Story = StoryObj<typeof TemplatePageHeader>;

export const CanUpdate: Story = {};

export const CanNotUpdate: Story = {
	args: {
		permissions: {
			canUpdateTemplate: false,
		},
	},
};

export const CannotCreateWorkspace: Story = {
	args: {
		workspacePermissions: {
			createWorkspaceForUser: false,
		},
	},
};

export const Deprecated: Story = {
	args: {
		template: {
			...MockTemplate,
			deprecated: true,
			deprecation_message:
				"This template is not going to be used anymore. [See details](#details).",
		},
	},
};
