import type { Meta, StoryObj } from "@storybook/react";
import { JobError } from "api/queries/templates";
import {
	MockProvisionerJob,
	MockTemplateVersion,
	MockWorkspaceBuildLogs,
} from "testHelpers/entities";
import { withProvisioners, withWebSocket } from "testHelpers/storybook";
import { BuildLogsDrawer } from "./BuildLogsDrawer";

const meta: Meta<typeof BuildLogsDrawer> = {
	title: "pages/CreateTemplatePage/BuildLogsDrawer",
	component: BuildLogsDrawer,
	args: {
		open: true,
	},
};

export default meta;
type Story = StoryObj<typeof BuildLogsDrawer>;

export const Loading: Story = {};

export const MissingVariables: Story = {
	args: {
		templateVersion: MockTemplateVersion,
		error: new JobError(
			{
				...MockProvisionerJob,
				error_code: "REQUIRED_TEMPLATE_VARIABLES",
			},
			MockTemplateVersion,
		),
	},
};

export const NoProvisioners: Story = {
	args: {
		templateVersion: {...MockTemplateVersion, organization_id: "org-id"},
	},
	decorators: [withProvisioners],
	parameters: {
		organization_id: "org-id",
		tags: MockTemplateVersion.job.tags,
		provisioners: [],
	}
};

export const ProvisionersUnhealthy: Story = {
	args: {
		templateVersion: {...MockTemplateVersion, organization_id: "org-id"},
	},
	decorators: [withProvisioners],
	parameters: {
		organization_id: "org-id",
		tags: MockTemplateVersion.job.tags,
		provisioners: [
			{
				last_seen_at: new Date(new Date().getTime() - 5 * 60 * 1000).toISOString()
			},
		],
	}
};

export const ProvisionersHealthy: Story = {
	args: {
		templateVersion: {...MockTemplateVersion, organization_id: "org-id"},
	},
	decorators: [withProvisioners],
	parameters: {
		organization_id: "org-id",
		tags: MockTemplateVersion.job.tags,
		provisioners: [
			{
				last_seen_at: new Date()
			},
		],
	}
};


export const Logs: Story = {
	args: {
		templateVersion: {
			...MockTemplateVersion,
			job: {
				...MockTemplateVersion.job,
				status: "running",
			},
		},
	},
	decorators: [withWebSocket],
	parameters: {
		webSocket: MockWorkspaceBuildLogs.map((log) => ({
			event: "message",
			data: JSON.stringify(log),
		})),
	},
};
