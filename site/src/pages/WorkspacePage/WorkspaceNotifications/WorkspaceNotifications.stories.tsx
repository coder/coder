import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { getWorkspaceResolveAutostartQueryKey } from "#/api/queries/workspaceQuota";
import type { Workspace } from "#/api/typesGenerated";
import type { WorkspacePermissions } from "#/modules/workspaces/permissions";
import {
	MockOutdatedWorkspace,
	MockTemplate,
	MockTemplateVersion,
	MockTemplateVersionWithMarkdownMessage,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceResource,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { WorkspaceNotifications } from "./WorkspaceNotifications";

export const defaultPermissions: WorkspacePermissions = {
	readWorkspace: true,
	shareWorkspace: true,
	updateWorkspace: true,
	updateWorkspaceVersion: true,
	deleteFailedWorkspace: true,
};

const meta: Meta<typeof WorkspaceNotifications> = {
	title: "pages/WorkspacePage/WorkspaceNotifications",
	component: WorkspaceNotifications,
	args: {
		latestVersion: MockTemplateVersion,
		template: MockTemplate,
		workspace: MockWorkspace,
		permissions: defaultPermissions,
	},
	decorators: [withDashboardProvider],
	parameters: {
		queries: [
			{
				key: getWorkspaceResolveAutostartQueryKey(MockOutdatedWorkspace.id),
				data: {
					parameter_mismatch: false,
				},
			},
		],
		features: ["advanced_template_scheduling"],
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceNotifications>;

export const Outdated: Story = {
	args: {
		workspace: MockOutdatedWorkspace,
	},

	play: async ({ step }) => {
		await step("activate click trigger", async () => {
			await userEvent.click(screen.getByTestId("info-notifications"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					MockTemplateVersion.message,
				),
			);
		});
	},
};

export const OutdatedWithMarkdownMessage: Story = {
	args: {
		workspace: MockOutdatedWorkspace,
		latestVersion: MockTemplateVersionWithMarkdownMessage,
	},

	play: async ({ step }) => {
		await step("activate click trigger", async () => {
			await userEvent.click(screen.getByTestId("info-notifications"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					/an update is available/i,
				),
			);
		});
	},
};

export const RequiresManualUpdate: Story = {
	args: {
		workspace: {
			...MockOutdatedWorkspace,
			automatic_updates: "always",
			autostart_schedule: "daily",
		},
	},
	parameters: {
		queries: [
			{
				key: getWorkspaceResolveAutostartQueryKey(MockOutdatedWorkspace.id),
				data: {
					parameter_mismatch: true,
				},
			},
		],
	},

	play: async ({ step }) => {
		await step("activate click trigger", async () => {
			await userEvent.click(screen.getByTestId("warning-notifications"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					/unable to automatically update/i,
				),
			);
		});
	},
};

/**
 * Creates a workspace with unhealthy agents using the given agent
 * overrides, for use in notification stories.
 */
function createUnhealthyWorkspace(
	agentOverrides: Partial<typeof MockWorkspaceAgent>,
): Workspace {
	const agent = { ...MockWorkspaceAgent, ...agentOverrides };
	return {
		...MockWorkspace,
		health: {
			healthy: false,
			failing_agents: [agent.id],
		},
		latest_build: {
			...MockWorkspace.latest_build,
			status: "running",
			resources: [
				{
					...MockWorkspaceResource,
					agents: [agent],
				},
			],
		},
	};
}

export const StartupScriptFailed: Story = {
	args: {
		workspace: createUnhealthyWorkspace({
			status: "connected",
			lifecycle_state: "start_error",
			health: { healthy: false },
		}),
	},

	play: async ({ step }) => {
		await step("shows startup script failure message", async () => {
			await userEvent.click(screen.getByTestId("warning-notifications"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					/a startup script has failed/i,
				),
			);
		});
		await step("does not offer restart", async () => {
			expect(
				screen.queryByRole("button", { name: /restart/i }),
			).not.toBeInTheDocument();
		});
	},
};

export const AgentDisconnected: Story = {
	args: {
		workspace: createUnhealthyWorkspace({
			status: "disconnected",
			lifecycle_state: "ready",
			health: { healthy: false },
		}),
	},

	play: async ({ step }) => {
		await step("activate click trigger", async () => {
			await userEvent.click(screen.getByTestId("warning-notifications"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					/one or more workspace agents need attention/i,
				),
			);
		});
	},
};

export const AgentTimeout: Story = {
	args: {
		workspace: createUnhealthyWorkspace({
			status: "timeout",
			lifecycle_state: "starting",
			health: { healthy: false },
		}),
	},

	play: async ({ step }) => {
		await step("activate click trigger", async () => {
			await userEvent.click(screen.getByTestId("warning-notifications"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					/one or more workspace agents need attention/i,
				),
			);
		});
	},
};

export const StartupScriptFailedWithoutUpdatePermission: Story = {
	args: {
		...StartupScriptFailed.args,
		permissions: {
			...defaultPermissions,
			updateWorkspace: false,
		},
	},

	play: StartupScriptFailed.play,
};

const DormantWorkspace = {
	...MockWorkspace,
	dormant_at: new Date("2020-01-01T00:00:00Z").toISOString(),
};

export const Dormant: Story = {
	args: {
		workspace: DormantWorkspace,
	},

	play: async ({ step }) => {
		await step("activate click trigger", async () => {
			await userEvent.click(screen.getByTestId("warning-notifications"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					/workspace is dormant/i,
				),
			);
		});
	},
};

export const DormantWithDeletingDate: Story = {
	args: {
		...Dormant.args,
		workspace: {
			...DormantWorkspace,
			deleting_at: new Date("2020-10-01T00:00:00Z").toISOString(),
		},
	},

	play: Dormant.play,
};

export const PendingInQueue: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				status: "pending",
				job: {
					...MockWorkspace.latest_build.job,
					queue_size: 10,
					queue_position: 3,
				},
			},
		},
	},

	play: async ({ step }) => {
		await step("activate click trigger", async () => {
			await userEvent.click(await screen.findByTestId("info-notifications"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					/build is pending/i,
				),
			);
		});
	},
};

export const TemplateDeprecated: Story = {
	args: {
		template: {
			...MockTemplate,
			deprecated: true,
			deprecation_message:
				"Template deprecated due to reasons. [Learn more](#)",
		},
	},

	play: async ({ step }) => {
		await step("activate click trigger", async () => {
			await userEvent.click(screen.getByTestId("warning-notifications"));
			await waitFor(() =>
				expect(screen.getByRole("dialog")).toHaveTextContent(
					/deprecated template/i,
				),
			);
		});
	},
};
