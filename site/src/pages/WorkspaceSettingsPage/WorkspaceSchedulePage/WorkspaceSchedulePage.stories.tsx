import type { Meta, StoryObj } from "@storybook/react-vite";
import { Outlet } from "react-router";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { templateByNameKey } from "#/api/queries/templates";
import { workspaceByOwnerAndNameKey } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import { Toaster } from "#/components/Toaster/Toaster";
import type { WorkspacePermissions } from "#/modules/workspaces/permissions";
import {
	MockPrebuiltWorkspace,
	MockTemplate,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceBuild,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import { WorkspaceSettingsLayout } from "../WorkspaceSettingsLayout";
import WorkspaceSchedulePage from "./WorkspaceSchedulePage";

const meta = {
	title: "pages/WorkspaceSchedulePage",
	component: WorkspaceSettingsLayout,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
	},
	beforeEach: () => {
		spyOn(API, "putWorkspaceAutostart").mockResolvedValue();
		spyOn(API, "putWorkspaceAutostop").mockResolvedValue();
	},
} satisfies Meta<typeof WorkspaceSchedulePage>;

export default meta;
type Story = StoryObj<typeof WorkspaceSchedulePage>;

export const RegularWorkspace: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(MockWorkspace),
		queries: workspaceQueries(MockWorkspace),
	},
};

export const PrebuiltWorkspace: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(MockPrebuiltWorkspace),
		queries: workspaceQueries(MockPrebuiltWorkspace),
	},
};

const autostopDisabledWorkspace: Workspace = { ...MockWorkspace, ttl_ms: 0 };

export const EnablingAutostopUsesTemplateDefault: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(autostopDisabledWorkspace),
		queries: workspaceQueries(autostopDisabledWorkspace),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const autostopToggle = await canvas.findByLabelText("Enable Autostop");
		await user.click(autostopToggle);
		await canvas.findByText("Your workspace will shut down 1 day after", {
			exact: false,
		});
	},
};

export const EnablingAutostopShowsRestartDialog: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(autostopDisabledWorkspace),
		queries: workspaceQueries(autostopDisabledWorkspace),
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			autostopDisabledWorkspace,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();
		await user.click(await canvas.findByLabelText("Enable Autostop"));
		await user.click(await canvas.findByRole("button", { name: /save/i }));
		await body.findByText(
			`Schedule for workspace "${MockWorkspace.name}" updated successfully.`,
		);
		await body.findByText("Restart workspace?");
	},
};

export const ApplyLaterKeepsUserOnSchedulePage: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(autostopDisabledWorkspace),
		queries: workspaceQueries(autostopDisabledWorkspace),
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			autostopDisabledWorkspace,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();
		const restartSpy = spyOn(API, "restartWorkspace");
		await user.click(await canvas.findByLabelText("Enable Autostop"));
		await user.click(await canvas.findByRole("button", { name: /save/i }));
		await body.findByText("Restart workspace?");
		await user.click(await body.findByRole("button", { name: /apply later/i }));
		// The dialog closes without restarting or leaving the schedule page.
		await waitFor(() =>
			expect(body.queryByText("Restart workspace?")).not.toBeInTheDocument(),
		);
		expect(restartSpy).not.toHaveBeenCalled();
		await canvas.findByLabelText("Enable Autostop");
	},
};

export const RestartFailureShowsErrorToast: Story = {
	parameters: {
		// Confirming the restart navigates to the workspace page, so this
		// story needs that route plus a layout-level Toaster that stays
		// mounted across the navigation, like the production app root.
		reactRouter: reactRouterParameters({
			location: {
				path: `/@${autostopDisabledWorkspace.owner_name}/${autostopDisabledWorkspace.name}/settings/schedule`,
			},
			routing: [
				{
					element: (
						<>
							<Outlet />
							<Toaster />
						</>
					),
					children: [
						...reactRouterOutlet(
							{ path: "/:username/:workspace/settings/schedule" },
							<WorkspaceSchedulePage />,
						),
						{
							path: "/:username/:workspace",
							element: <div>Workspace Page</div>,
						},
					],
				},
			],
		}),
		queries: workspaceQueries(autostopDisabledWorkspace),
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			autostopDisabledWorkspace,
		);
		spyOn(API, "restartWorkspace").mockRejectedValue(
			new Error(
				"The workspace stopped, but the server did not start it again.",
			),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();
		await user.click(await canvas.findByLabelText("Enable Autostop"));
		await user.click(await canvas.findByRole("button", { name: /save/i }));
		await body.findByText("Restart workspace?");
		await user.click(await body.findByRole("button", { name: /^restart$/i }));
		await body.findByText(
			`Failed to restart workspace "${MockWorkspace.name}".`,
		);
	},
};

export const ChangingAutostopValueShowsRestartDialog: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(MockWorkspace),
		queries: workspaceQueries(MockWorkspace),
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(MockWorkspace);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();
		const ttlInput = await canvas.findByLabelText(
			"Time until shutdown (hours)",
		);
		await user.clear(ttlInput);
		await user.type(ttlInput, "4");
		await user.click(await canvas.findByRole("button", { name: /save/i }));
		await body.findByText(
			`Schedule for workspace "${MockWorkspace.name}" updated successfully.`,
		);
		await body.findByText("Restart workspace?");
	},
};

export const RestartDialogWarnsAboutTemplateUpdate: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(MockWorkspace),
		queries: workspaceQueries(MockWorkspace),
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(MockWorkspace);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();
		const ttlInput = await canvas.findByLabelText(
			"Time until shutdown (hours)",
		);
		await user.clear(ttlInput);
		await user.type(ttlInput, "4");
		await user.click(await canvas.findByRole("button", { name: /save/i }));
		await body.findByText("Restart workspace?");
		await body.findByText(
			/The restarted workspace will use the template's active version/,
		);
	},
};

export const DisablingAutostopSkipsRestartDialog: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(MockWorkspace),
		queries: workspaceQueries(MockWorkspace),
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(MockWorkspace);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();
		// MockWorkspace has autostop enabled, so clicking the toggle disables it.
		await user.click(await canvas.findByLabelText("Enable Autostop"));
		await user.click(await canvas.findByRole("button", { name: /save/i }));
		await body.findByText(
			`Schedule for workspace "${MockWorkspace.name}" updated successfully.`,
		);
		expect(body.queryByText("Restart workspace?")).not.toBeInTheDocument();
	},
};

const stoppedAutostopDisabledWorkspace: Workspace = {
	...autostopDisabledWorkspace,
	latest_build: { ...MockWorkspaceBuild, status: "stopped" },
};

export const EnablingAutostopWhileStoppedSkipsDialog: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(stoppedAutostopDisabledWorkspace),
		queries: workspaceQueries(stoppedAutostopDisabledWorkspace),
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			stoppedAutostopDisabledWorkspace,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();
		await user.click(await canvas.findByLabelText("Enable Autostop"));
		await user.click(await canvas.findByRole("button", { name: /save/i }));
		await body.findByText(
			`Schedule for workspace "${MockWorkspace.name}" updated successfully.`,
		);
		expect(body.queryByText("Restart workspace?")).not.toBeInTheDocument();
	},
};

export const ChangingOnlyAutostartSkipsDialog: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(MockWorkspace),
		queries: workspaceQueries(MockWorkspace),
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(MockWorkspace);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		const user = userEvent.setup();
		await user.click(await canvas.findByLabelText("Enable Autostart"));
		await user.click(await canvas.findByRole("button", { name: /save/i }));
		await body.findByText(
			`Schedule for workspace "${MockWorkspace.name}" updated successfully.`,
		);
		expect(body.queryByText("Restart workspace?")).not.toBeInTheDocument();
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
				path: "/:username/:workspace/settings/schedule",
			},
			<WorkspaceSchedulePage />,
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
			key: ["workspaces", workspace.id, "permissions"],
			data: {
				readWorkspace: true,
				shareWorkspace: true,
				updateWorkspace: true,
				updateWorkspaceVersion: true,
				deleteFailedWorkspace: true,
			} satisfies WorkspacePermissions,
		},
		{
			key: templateByNameKey(
				MockWorkspace.organization_id,
				MockWorkspace.template_name,
			),
			data: MockTemplate,
		},
	];
}
