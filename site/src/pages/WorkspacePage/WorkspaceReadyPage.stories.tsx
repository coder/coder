import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { buildInfoKey } from "#/api/queries/buildInfo";
import { templateVersion } from "#/api/queries/templates";
import { agentListeningPorts } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import * as Mocks from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import type { WorkspacePermissions } from "../../modules/workspaces/permissions";
import { WorkspaceReadyPage } from "./WorkspaceReadyPage";

const permissions: WorkspacePermissions = {
	readWorkspace: true,
	shareWorkspace: true,
	updateWorkspace: true,
	updateWorkspaceVersion: true,
	deleteFailedWorkspace: true,
};

const workspaceQueries = (workspace: Workspace) => [
	{ key: buildInfoKey, data: Mocks.MockBuildInfo },
	{
		key: agentListeningPorts(Mocks.MockWorkspaceAgent.id).queryKey,
		data: Mocks.MockListeningPortsResponse,
	},
	{
		key: templateVersion(workspace.template_active_version_id).queryKey,
		data: Mocks.MockTemplateVersion,
	},
];

const meta: Meta<typeof WorkspaceReadyPage> = {
	title: "pages/WorkspacePage/WorkspaceReadyPage",
	component: WorkspaceReadyPage,
	args: {
		workspace: Mocks.MockWorkspace,
		template: Mocks.MockTemplate,
		permissions,
	},
	parameters: {
		queries: workspaceQueries(Mocks.MockWorkspace),
		user: Mocks.MockUserOwner,
	},
	decorators: [withAuthProvider, withDashboardProvider, withProxyProvider()],
	beforeEach: () => {
		spyOn(API, "getDynamicParameters").mockResolvedValue([]);
		spyOn(API, "workspaceBuildTimings").mockResolvedValue({
			provisioner_timings: [],
			agent_script_timings: [],
			agent_connection_timings: [],
		});
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceReadyPage>;

const openRestartDialog = async (canvasElement: HTMLElement) => {
	const body = within(canvasElement.ownerDocument.body);
	const restartButton = await body.findByRole("button", {
		name: /^restart…$/i,
	});
	await userEvent.click(restartButton);
	return await body.findByRole("dialog");
};

export const RestartDialog: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await openRestartDialog(canvasElement);
		await waitFor(() =>
			expect(dialog).toHaveTextContent(/delete non-persistent data/),
		);
		expect(dialog).toHaveTextContent(
			/The workspace will start using the template's active version/,
		);
	},
};

export const RestartDialogOutdatedWorkspace: Story = {
	args: {
		workspace: Mocks.MockRunningOutdatedWorkspace,
	},
	parameters: {
		queries: workspaceQueries(Mocks.MockRunningOutdatedWorkspace),
	},
	play: async ({ canvasElement }) => {
		const dialog = await openRestartDialog(canvasElement);
		await waitFor(() =>
			expect(dialog).toHaveTextContent(
				/The workspace will start using the template's active version/,
			),
		);
	},
};

export const ConfirmRestart: Story = {
	beforeEach: () => {
		spyOn(API, "restartWorkspace").mockResolvedValue(undefined);
	},
	play: async ({ canvasElement }) => {
		const dialog = await openRestartDialog(canvasElement);
		await userEvent.click(
			within(dialog).getByRole("button", { name: "Restart" }),
		);
		await waitFor(() =>
			expect(API.restartWorkspace).toHaveBeenCalledWith(
				expect.objectContaining({ workspace: Mocks.MockWorkspace }),
			),
		);
	},
};
