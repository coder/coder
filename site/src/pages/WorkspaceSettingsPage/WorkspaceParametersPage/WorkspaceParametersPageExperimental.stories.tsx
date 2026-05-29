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
	MockWorkspaceBuildParameter4,
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

// Regression test for coder/coder#25822. When a new template version adds
// stricter validation, conditional logic, or option changes that the
// already-stored immutable value no longer satisfies, the parameters page
// must not block the user from editing unrelated mutable parameters. The
// existing immutable value is carried over by the backend resolver, so
// surfacing diagnostics for an unchanged immutable parameter would block
// legitimate updates.
export const UnchangedImmutableParameterDoesNotBlock: Story = {
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify({
					id: 0,
					diagnostics: [],
					parameters: [
						{
							...MockPreviewParameter,
							name: MockWorkspaceBuildParameter1.name,
							value: { valid: true, value: MockWorkspaceBuildParameter1.value },
						},
						{
							...MockPreviewParameter,
							name: MockWorkspaceBuildParameter4.name,
							display_name: "Pre-existing immutable parameter",
							mutable: false,
							value: { valid: true, value: MockWorkspaceBuildParameter4.value },
							diagnostics: [
								{
									severity: "error",
									summary: "Value no longer matches updated options",
									detail:
										"The template version updated the allowed options for this parameter.",
									extra: { code: "" },
								},
							],
						},
					],
				}),
			},
		],
		queries: [
			...workspaceQueries(MockWorkspace).filter(
				(q) =>
					q.key[0] !==
					workspaceBuildParametersKey(MockWorkspace.latest_build.id)[0],
			),
			{
				key: workspaceBuildParametersKey(MockWorkspace.latest_build.id),
				data: [MockWorkspaceBuildParameter1, MockWorkspaceBuildParameter4],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The blocking alert from #25822 must not appear when the only
		// parameter with diagnostics is an unchanged immutable parameter.
		await waitFor(() =>
			expect(
				canvas.queryByText("Workspace update blocked"),
			).not.toBeInTheDocument(),
		);
		// The submit button stays enabled so the user can update mutable
		// parameters; the backend resolver keeps the existing immutable
		// value when nothing about it changed.
		const submit = await canvas.findByRole("button", {
			name: "Update and restart",
		});
		expect(submit).toBeEnabled();
	},
};

// Companion to UnchangedImmutableParameterDoesNotBlock. When the immutable
// parameter's rendered value differs from the autofill (e.g. the user did
// try to change it, or the template substituted a different value), the
// blocking alert is still required so the build doesn't fail with an
// immutable-parameter error from the resolver.
export const ChangedImmutableParameterStillBlocks: Story = {
	parameters: {
		webSocket: [
			{
				event: "message",
				data: JSON.stringify({
					id: 0,
					diagnostics: [],
					parameters: [
						{
							...MockPreviewParameter,
							name: MockWorkspaceBuildParameter4.name,
							display_name: "Pre-existing immutable parameter",
							mutable: false,
							value: { valid: true, value: "changed-value" },
							diagnostics: [
								{
									severity: "error",
									summary: "Immutable parameter changed",
									detail: "Immutable parameters cannot change after creation.",
									extra: { code: "" },
								},
							],
						},
					],
				}),
			},
		],
		queries: [
			...workspaceQueries(MockWorkspace).filter(
				(q) =>
					q.key[0] !==
					workspaceBuildParametersKey(MockWorkspace.latest_build.id)[0],
			),
			{
				key: workspaceBuildParametersKey(MockWorkspace.latest_build.id),
				data: [MockWorkspaceBuildParameter4],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("Workspace update blocked");
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
