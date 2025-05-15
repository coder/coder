import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, within } from "@storybook/test";
import { deploymentConfigQueryKey } from "api/queries/deployment";
import { agentLogsKey, buildLogsKey } from "api/queries/workspaces";
import * as Mocks from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withDesktopViewport,
} from "testHelpers/storybook";
import { WorkspaceActions } from "./WorkspaceActions";

const meta: Meta<typeof WorkspaceActions> = {
	title: "pages/WorkspacePage/WorkspaceActions",
	component: WorkspaceActions,
	args: {
		isUpdating: false,
		permissions: {
			deleteFailedWorkspace: true,
			deploymentConfig: true,
			readWorkspace: true,
			updateWorkspace: true,
			updateWorkspaceVersion: true,
		},
	},
	decorators: [withDashboardProvider, withDesktopViewport, withAuthProvider],
	parameters: {
		user: Mocks.MockUserOwner,
		queries: [
			{
				key: deploymentConfigQueryKey,
				data: Mocks.MockDeploymentConfig,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceActions>;

export const Starting: Story = {
	args: {
		workspace: Mocks.MockStartingWorkspace,
	},
};

export const Running: Story = {
	args: {
		workspace: Mocks.MockWorkspace,
	},
};

export const RunningUpdateAvailable: Story = {
	name: "Running (Update available)",
	args: {
		workspace: {
			...Mocks.MockWorkspace,
			outdated: true,
		},
	},
};

export const RunningRequireActiveVersion: Story = {
	name: "Running (No required update)",
	args: {
		workspace: {
			...Mocks.MockWorkspace,
			template_require_active_version: true,
		},
	},
};

export const RunningUpdateRequired: Story = {
	name: "Running (Update Required)",
	args: {
		workspace: {
			...Mocks.MockWorkspace,
			template_require_active_version: true,
			outdated: true,
		},
	},
};

export const Stopping: Story = {
	args: {
		workspace: Mocks.MockStoppingWorkspace,
	},
};

export const Stopped: Story = {
	args: {
		workspace: Mocks.MockStoppedWorkspace,
	},
};

export const StoppedUpdateAvailable: Story = {
	name: "Stopped (Update available)",
	args: {
		workspace: {
			...Mocks.MockStoppedWorkspace,
			outdated: true,
		},
	},
};

export const StoppedRequireActiveVersion: Story = {
	name: "Stopped (No required update)",
	args: {
		workspace: {
			...Mocks.MockStoppedWorkspace,
			template_require_active_version: true,
		},
	},
};

export const StoppedUpdateRequired: Story = {
	name: "Stopped (Update Required)",
	args: {
		workspace: {
			...Mocks.MockStoppedWorkspace,
			template_require_active_version: true,
			outdated: true,
		},
	},
};

export const Updating: Story = {
	args: {
		workspace: Mocks.MockOutdatedWorkspace,
		isUpdating: true,
	},
};

export const Restarting: Story = {
	args: {
		workspace: Mocks.MockStoppingWorkspace,
		isRestarting: true,
	},
};

export const Canceling: Story = {
	args: {
		workspace: Mocks.MockCancelingWorkspace,
	},
};

export const Deleting: Story = {
	args: {
		workspace: Mocks.MockDeletingWorkspace,
	},
};

export const Deleted: Story = {
	args: {
		workspace: Mocks.MockDeletedWorkspace,
	},
};

export const Outdated: Story = {
	args: {
		workspace: Mocks.MockOutdatedWorkspace,
	},
};

export const Failed: Story = {
	args: {
		workspace: Mocks.MockFailedWorkspace,
	},
};

export const FailedWithDebug: Story = {
	args: {
		workspace: Mocks.MockFailedWorkspace,
		permissions: {
			deploymentConfig: true,
			deleteFailedWorkspace: true,
			readWorkspace: true,
			updateWorkspace: true,
			updateWorkspaceVersion: true,
		},
	},
};

export const CancelShownForOwner: Story = {
	args: {
		workspace: {
			...Mocks.MockStartingWorkspace,
			template_allow_user_cancel_workspace_jobs: false,
		},
	},
};

export const CancelShownForUser: Story = {
	args: {
		workspace: Mocks.MockStartingWorkspace,
	},
	parameters: {
		user: Mocks.MockUserMember,
	},
};

export const CancelHiddenForUser: Story = {
	args: {
		workspace: {
			...Mocks.MockStartingWorkspace,
			template_allow_user_cancel_workspace_jobs: false,
		},
	},
	parameters: {
		user: Mocks.MockUserMember,
	},
};

export const OpenDownloadLogs: Story = {
	args: {
		workspace: Mocks.MockWorkspace,
	},
	parameters: {
		queries: [
			{
				key: buildLogsKey(Mocks.MockWorkspace.id),
				data: generateLogs(200),
			},
			{
				key: agentLogsKey(Mocks.MockWorkspaceAgent.id),
				data: generateLogs(400),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Workspace actions" }),
		);
		const screen = within(document.body);
		await userEvent.click(screen.getByText("Download logs…"));
		await expect(screen.getByTestId("dialog")).toBeInTheDocument();
	},
};

export const CanDeleteDormantWorkspace: Story = {
	args: {
		workspace: Mocks.MockDormantWorkspace,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Workspace actions" }),
		);
		const screen = within(document.body);
		const deleteButton = screen.getByText("Delete…");
		await expect(deleteButton).toBeEnabled();
	},
};

function generateLogs(count: number) {
	return Array.from({ length: count }, (_, i) => ({
		output: `log ${i + 1}`,
	}));
}
