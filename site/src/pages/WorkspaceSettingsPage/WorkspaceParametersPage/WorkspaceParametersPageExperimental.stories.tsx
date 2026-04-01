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
import WorkspaceParametersPageExperimental from "./WorkspaceParametersPageExperimental";

const meta = {
	title: "pages/WorkspaceParametersPageExperimental",
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
				event: "message",
				data: JSON.stringify({
					id: 0,
					diagnostics: [],
					parameters: [MockPreviewParameter, MockDropdownParameter],
				}),
			},
		],
	},
} satisfies Meta<typeof WorkspaceParametersPageExperimental>;

export default meta;
type Story = StoryObj<typeof WorkspaceParametersPageExperimental>;

export const NoParameters: Story = {
	parameters: {
		webSocket: [
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
			<WorkspaceParametersPageExperimental />,
		),
	});
}

function workspaceQueries(workspace: Workspace) {
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
			} satisfies WorkspacePermissions,
		},
	];
}

function filledWebSocketParams(): WebSocketEvent[] {
	return [
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
