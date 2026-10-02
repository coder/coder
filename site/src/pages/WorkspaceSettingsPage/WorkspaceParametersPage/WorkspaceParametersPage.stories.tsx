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

export const RequireActiveVersionUpdating: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(
			MockOutdatedStoppedWorkspaceRequireActiveVersion,
			{
				templateVersionId:
					MockOutdatedStoppedWorkspaceRequireActiveVersion.template_active_version_id,
			},
		),
		queries: workspaceQueries(
			MockOutdatedStoppedWorkspaceRequireActiveVersion,
			{ updateWorkspaceVersion: false },
		),
		webSocket: filledWebSocketParams(),
	},
};

// The template dropped the option this workspace had selected. The backend
// falls back to the default and reports the substitution as a warning.
export const StaleOptionWarning: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(
			MockOutdatedStoppedWorkspaceRequireActiveVersion,
			{
				templateVersionId:
					MockOutdatedStoppedWorkspaceRequireActiveVersion.template_active_version_id,
			},
		),
		queries: workspaceQueries(
			MockOutdatedStoppedWorkspaceRequireActiveVersion,
			{ updateWorkspaceVersion: false },
		),
		webSocket: staleOptionWebSocketParams(),
	},
};

// The backend never substitutes an immutable parameter's value, so the stale
// value is kept, fails option validation, and blocks the update.
export const StaleOptionOnImmutableParameter: Story = {
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
					parameters: [
						{
							...MockDropdownParameter,
							mutable: false,
							value: { value: "t2.nano", valid: true },
							diagnostics: [
								{
									severity: "error",
									summary: "Value must be a valid option",
									detail:
										'the value "t2.nano" must be defined as one of options',
									extra: { code: "" },
								},
							],
						},
					],
				}),
			},
		],
	},
};

function workspaceRouterParameters(
	workspace: Workspace,
	searchParams?: Record<string, string>,
) {
	return reactRouterParameters({
		location: {
			pathParams: {
				username: `@${workspace.owner_name}`,
				workspace: workspace.name,
			},
			searchParams,
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

function staleOptionWebSocketParams(): WebSocketEvent[] {
	const staleParams = JSON.stringify({
		id: 0,
		diagnostics: [],
		parameters: [
			{
				...MockDropdownParameter,
				value: MockDropdownParameter.default_value,
				diagnostics: [
					{
						severity: "warning",
						summary: "Previously selected option is no longer available",
						detail: 'The value "t2.nano" is not one of the available options.',
						extra: { code: "stale_option" },
					},
				],
			},
		],
	});
	return [
		{
			event: "open",
		},
		{
			event: "message",
			data: staleParams,
		},
	];
}
