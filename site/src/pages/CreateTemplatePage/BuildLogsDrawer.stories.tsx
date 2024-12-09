import type { Meta, StoryObj } from "@storybook/react";
import { JobError } from "api/queries/templates";
import {
	MockProvisionerJob,
	MockTemplateVersion,
	MockWorkspaceBuildLogs,
} from "testHelpers/entities";
import { withWebSocket } from "testHelpers/storybook";
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
		templateVersion: {
			...MockTemplateVersion,
			matched_provisioners: {
				count: 0,
				available: 0,
			},
		},
	},
};

export const ProvisionersUnhealthy: Story = {
	args: {
		templateVersion: {
			...MockTemplateVersion,
			matched_provisioners: {
				count: 1,
				available: 0,
			},
		},
	},
};

export const ProvisionersHealthy: Story = {
	args: {
		templateVersion: {
			...MockTemplateVersion,
			matched_provisioners: {
				count: 1,
				available: 1,
			},
		},
	},
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
