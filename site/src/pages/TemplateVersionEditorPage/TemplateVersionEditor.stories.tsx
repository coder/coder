import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, userEvent, within } from "storybook/test";
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
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { TemplateVersionEditor } from "./TemplateVersionEditor";

const meta: Meta<typeof TemplateVersionEditor> = {
	title: "pages/TemplateVersionEditor",
	component: TemplateVersionEditor,
	decorators: [withDashboardProvider],
	parameters: {
		layout: "fullscreen",
	},
	args: {
		activePath: "main.tf",
		template: MockTemplate,
		templateVersion: MockTemplateVersion,
		fileTree: MockTemplateVersionFileTree,
		onFileTreeChange: action("onFileTreeChange"),
		onPublish: action("onPublish"),
		onConfirmPublish: action("onConfirmPublish"),
		onCancelPublish: action("onCancelPublish"),
		createWorkspaceUrl: "/templates/default/test-template/workspace",
		onSubmitMissingVariableValues: action("onSubmitMissingVariableValues"),
		onCancelSubmitMissingVariableValues: action(
			"onCancelSubmitMissingVariableValues",
		),
		provisionerTags: { wibble: "wobble", wiggle: "woggle" },
	},
};

export default meta;
type Story = StoryObj<typeof TemplateVersionEditor>;

export const Example: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const backLink = canvas.getByRole("link", { name: "Back to the template" });
		await expect(backLink).toBeVisible();
	},
};

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

export const PublishDialog: Story = {
	args: {
		isAskingPublishParameters: true,
	},
};

export const PublishDialogActiveVersionHelp: Story = {
	args: {
		isAskingPublishParameters: true,
	},
	play: async ({ canvasElement }) => {
		// The dialog and popover are portaled, so query against the document body.
		const body = within(canvasElement.ownerDocument.body);
		const trigger = await body.findByRole("button", { name: "More info" });
		await userEvent.click(trigger);
		await expect(await body.findByText("Active versions")).toBeInTheDocument();
		await expect(
			body.getByRole("link", { name: "Review the documentation" }),
		).toBeInTheDocument();
	},
};

export const Published = {
	args: {
		publishedVersion: MockTemplateVersion,
	},
};
