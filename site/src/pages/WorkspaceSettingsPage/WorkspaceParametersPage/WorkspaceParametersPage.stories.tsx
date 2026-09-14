import type { Meta, StoryObj, WebSocketEvent } from "@storybook/react-vite";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { workspaceBuildParametersKey } from "#/api/queries/workspaceBuilds";
import { workspaceByOwnerAndNameKey } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import type { WorkspacePermissions } from "#/modules/workspaces/permissions";
import {
	MockDropdownParameter,
	MockOutdatedRunningWorkspaceRequireActiveVersion,
	MockOutdatedStoppedWorkspaceRequireActiveVersion,
	MockPermissions,
	MockPreviewParameter,
	MockStoppedWorkspace,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceBuildParameter1,
	MockWorkspaceBuildParameter2,
	MockWorkspaceBuildParameter3,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withWebSocket,
} from "#/testHelpers/storybook";
import { WorkspaceSettingsLayout } from "../WorkspaceSettingsLayout";
import WorkspaceParametersPage from "./WorkspaceParametersPage";

const meta = {
	title: "pages/WorkspaceParametersPage",
	component: WorkspaceSettingsLayout,
	decorators: [withAuthProvider, withDashboardProvider, withWebSocket],
	args: {
		permissions: MockPermissions,
	},
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		reactRouter: workspaceRouterParameters(MockWorkspace),
		queries: workspaceQueries(MockWorkspace),
		webSocket: [
			{
				event: "open",
			},
			{
				event: "message",
				data: JSON.stringify({
					id: 0,
					diagnostics: [],
					parameters: [MockPreviewParameter, MockDropdownParameter],
				}),
			},
		],
	},
} satisfies Meta<typeof WorkspaceParametersPage>;

export default meta;
type Story = StoryObj<typeof WorkspaceParametersPage>;

export const NoParameters: Story = {
	parameters: {
		webSocket: [
			{
				event: "open",
			},
			{
				event: "message",
				data: JSON.stringify({
					id: 0,
					diagnostics: [],
					parameters: [],
				}),
			},
		],
	},
};

export const Parameters: Story = {};

export const Required: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Update and restart" }),
		);
	},
};

export const ShowConfirmation: Story = {
	beforeEach: () => {
		spyOn(API, "stopWorkspace").mockRejectedValue(
			new Error("would have stopped"),
		);
	},
	parameters: {
		webSocket: filledWebSocketParams(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Update and restart" }),
		);
	},
};

export const RestartWorkspace: Story = {
	beforeEach: () => {
		spyOn(API, "stopWorkspace").mockRejectedValue(
			new Error("would have stopped"),
		);
	},
	parameters: {
		webSocket: filledWebSocketParams(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Update and restart" }),
		);
		await userEvent.click(
			await screen.findByRole("button", { name: "Restart" }),
		);
		await waitFor(() =>
			expect(screen.getByText("would have stopped")).toBeInTheDocument(),
		);
	},
};

export const StartWorkspace: Story = {
	beforeEach: () => {
		spyOn(API, "stopWorkspace").mockRejectedValue(
			new Error("should not hit this"),
		);
		spyOn(API, "postWorkspaceBuild").mockRejectedValue(
			new Error("would have started"),
		);
	},
	parameters: {
		reactRouter: workspaceRouterParameters(MockStoppedWorkspace),
		queries: workspaceQueries(MockStoppedWorkspace),
		webSocket: filledWebSocketParams(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Update and start" }),
		);
		await waitFor(() =>
			expect(screen.getByText("would have started")).toBeInTheDocument(),
		);
	},
};

export const RequireActiveVersionBlocked: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(
			MockOutdatedStoppedWorkspaceRequireActiveVersion,
		),
		queries: workspaceQueries(
			MockOutdatedStoppedWorkspaceRequireActiveVersion,
			{ updateWorkspaceVersion: false },
		),
		webSocket: filledWebSocketParams(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(
				canvas.getByText(/requires automatic updates/),
			).toBeInTheDocument(),
		);
		const submitButton = canvas.getByRole("button", {
			name: "Update and start",
		});
		expect(submitButton).toBeDisabled();
	},
};

export const RequireActiveVersionBlockedRunning: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(
			MockOutdatedRunningWorkspaceRequireActiveVersion,
		),
		queries: workspaceQueries(
			MockOutdatedRunningWorkspaceRequireActiveVersion,
			{ updateWorkspaceVersion: false },
		),
		webSocket: filledWebSocketParams(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(
				canvas.getByText(/requires automatic updates/),
			).toBeInTheDocument(),
		);
		const submitButton = canvas.getByRole("button", {
			name: "Update and restart",
		});
		expect(submitButton).toBeDisabled();
	},
};

export const RequireActiveVersionEditable: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(
			MockOutdatedStoppedWorkspaceRequireActiveVersion,
		),
		queries: workspaceQueries(
			MockOutdatedStoppedWorkspaceRequireActiveVersion,
			{ updateWorkspaceVersion: true },
		),
		webSocket: filledWebSocketParams(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Wait for the form to render before asserting absence of warning.
		const submitButton = await canvas.findByRole("button", {
			name: "Update and start",
		});
		expect(
			canvas.queryByText(/requires automatic updates/),
		).not.toBeInTheDocument();
		expect(submitButton).not.toBeDisabled();
	},
};

function workspaceRouterParameters(workspace: Workspace) {
	return reactRouterParameters({
		location: {
			pathParams: {
				username: `@${workspace.owner_name}`,
				workspace: workspace.name,
			},
		},
		routing: reactRouterOutlet(
			{
				path: "/:username/:workspace/settings/parameters",
			},
			<WorkspaceParametersPage />,
		),
	});
}

function workspaceQueries(
	workspace: Workspace,
	permissionOverrides?: Partial<WorkspacePermissions>,
) {
	return [
		{
			key: workspaceByOwnerAndNameKey(workspace.owner_name, workspace.name),
			data: workspace,
		},
		{
			key: workspaceBuildParametersKey(workspace.latest_build.id),
			data: [
				MockWorkspaceBuildParameter1,
				MockWorkspaceBuildParameter2,
				MockWorkspaceBuildParameter3,
			],
		},
		{
			key: ["workspaces", workspace.id, "permissions"],
			data: {
				readWorkspace: true,
				shareWorkspace: true,
				updateWorkspace: true,
				updateWorkspaceVersion: true,
				deleteFailedWorkspace: true,
				...permissionOverrides,
			} satisfies WorkspacePermissions,
		},
	];
}

function filledWebSocketParams(): WebSocketEvent[] {
	return [
		{
			event: "open",
		},
		{
			event: "message",
			data: JSON.stringify({
				id: 0,
				diagnostics: [],
				parameters: [
					{
						...MockPreviewParameter,
						value: { valid: true, value: "test" },
					},
					MockDropdownParameter,
				],
			}),
		},
	];
}
