import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import {
	MockFailedProvisionerJob,
	MockRunningProvisionerJob,
	MockTemplate,
	MockTemplateVersion,
	MockTemplateVersionFileTree,
	MockWorkspaceBuildLogs,
	MockWorkspaceContainerResource,
	MockWorkspaceExtendedBuildLogs,
	MockWorkspaceImageResource,
	MockWorkspaceResource,
	MockWorkspaceResourceMultipleAgents,
	MockWorkspaceResourceSensitive,
	MockWorkspaceVolumeResource,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { TemplateVersionEditor } from "./TemplateVersionEditor";

const meta: Meta<typeof TemplateVersionEditor> = {
	title: "pages/TemplateVersionEditor",
	component: TemplateVersionEditor,
	decorators: [withDashboardProvider],
	parameters: {
		chromatic,
		layout: "fullscreen",
	},
	args: {
		activePath: "main.tf",
		template: MockTemplate,
		templateVersion: MockTemplateVersion,
		defaultFileTree: MockTemplateVersionFileTree,
		onPublish: action("onPublish"),
		onConfirmPublish: action("onConfirmPublish"),
		onCancelPublish: action("onCancelPublish"),
		onCreateWorkspace: action("onCreateWorkspace"),
		onSubmitMissingVariableValues: action("onSubmitMissingVariableValues"),
		onCancelSubmitMissingVariableValues: action(
			"onCancelSubmitMissingVariableValues",
		),
		provisionerTags: { wibble: "wobble", wiggle: "woggle" },
	},
};

export default meta;
type Story = StoryObj<typeof TemplateVersionEditor>;

export const Example: Story = {};

export const UndefinedLogs: Story = {
	args: {
		defaultTab: "logs",
		buildLogs: undefined,
		templateVersion: {
			...MockTemplateVersion,
			job: MockRunningProvisionerJob,
		},
	},
};

export const EmptyLogs: Story = {
	args: {
		defaultTab: "logs",
		buildLogs: [],
		templateVersion: {
			...MockTemplateVersion,
			job: MockRunningProvisionerJob,
		},
	},
};

export const NoProvisioners: Story = {
	args: {
		defaultTab: "logs",
		buildLogs: [],
		templateVersion: {
			...MockTemplateVersion,
			job: MockRunningProvisionerJob,
			matched_provisioners: {
				count: 0,
				available: 0,
			},
		},
	},
};

export const UnavailableProvisioners: Story = {
	args: {
		defaultTab: "logs",
		buildLogs: [],
		templateVersion: {
			...MockTemplateVersion,
			job: MockRunningProvisionerJob,
			matched_provisioners: {
				count: 1,
				available: 0,
			},
		},
	},
};

export const HealthyProvisioners: Story = {
	args: {
		defaultTab: "logs",
		buildLogs: [],
		templateVersion: {
			...MockTemplateVersion,
			job: MockRunningProvisionerJob,
			matched_provisioners: {
				count: 1,
				available: 1,
			},
		},
	},
};

export const Logs: Story = {
	args: {
		defaultTab: "logs",
		buildLogs: MockWorkspaceBuildLogs,
		templateVersion: {
			...MockTemplateVersion,
			job: MockRunningProvisionerJob,
		},
	},
};

export const Resources: Story = {
	args: {
		defaultTab: "resources",
		buildLogs: MockWorkspaceBuildLogs,
		resources: [
			MockWorkspaceResource,
			MockWorkspaceResourceSensitive,
			MockWorkspaceResourceMultipleAgents,
			MockWorkspaceVolumeResource,
			MockWorkspaceImageResource,
			MockWorkspaceContainerResource,
		],
	},
};

export const WithError = {
	args: {
		defaultTab: "logs",
		templateVersion: {
			...MockTemplateVersion,
			job: {
				...MockFailedProvisionerJob,
				error:
					"template import provision for start: terraform plan: exit status 1",
			},
		},
		buildLogs: MockWorkspaceExtendedBuildLogs,
	},
};

export const Published = {
	args: {
		publishedVersion: MockTemplateVersion,
	},
};
