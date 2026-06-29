import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { templateByNameKey } from "#/api/queries/templates";
import { workspaceByOwnerAndNameKey } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
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

export const ChangingAutostopShowsRestartDialog: Story = {
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
		await user.click(await canvas.findByLabelText("Enable Autostop"));
		await user.click(await canvas.findByRole("button", { name: /save/i }));
		await body.findByText(
			`Schedule for workspace "${MockWorkspace.name}" updated successfully.`,
		);
		await body.findByText("Restart workspace?");
	},
};

const stoppedWorkspace: Workspace = {
	...MockWorkspace,
	latest_build: { ...MockWorkspaceBuild, status: "stopped" },
};

export const ChangingAutostopWhileStoppedSkipsDialog: Story = {
	parameters: {
		reactRouter: workspaceRouterParameters(stoppedWorkspace),
		queries: workspaceQueries(stoppedWorkspace),
	},
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			stoppedWorkspace,
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
