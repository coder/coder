import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor } from "storybook/test";
import { JobError } from "#/api/queries/templates";
import {
	MockProvisionerJob,
	MockTemplateVersion,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import { withWebSocket } from "#/testHelpers/storybook";
import { BuildLogsDrawer } from "./BuildLogsDrawer";

const meta: Meta<typeof BuildLogsDrawer> = {
	title: "pages/CreateTemplatePage/BuildLogsDrawer",
	component: BuildLogsDrawer,
	args: {
		open: true,
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof BuildLogsDrawer>;

export const Loading: Story = {};

export const CloseWithButton: Story = {
	play: async ({ args }) => {
		// The drawer portals its content onto `document.body`, so query the screen.
		await userEvent.click(
			screen.getByRole("button", { name: "Close build logs" }),
		);
		await waitFor(() => expect(args.onClose).toHaveBeenCalled());
	},
};

export const CloseWithEscape: Story = {
	play: async ({ args }) => {
		await userEvent.keyboard("{Escape}");
		await waitFor(() => expect(args.onClose).toHaveBeenCalled());
	},
};

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
			matched_provisioners: {
				count: 1,
				available: 1,
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
