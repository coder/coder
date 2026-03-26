import {
	MockPrebuiltWorkspace,
	MockTemplate,
	MockUserOwner,
	MockWorkspace,
} from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorkspacePermissions } from "modules/workspaces/permissions";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { templateByNameKey } from "#/api/queries/templates";
import { workspaceByOwnerAndNameKey } from "#/api/queries/workspaces";
import type { Workspace } from "#/api/typesGenerated";
import { WorkspaceSettingsLayout } from "../WorkspaceSettingsLayout";
import WorkspaceSchedulePage from "./WorkspaceSchedulePage";

const meta = {
	title: "pages/WorkspaceSchedulePage",
	component: WorkspaceSettingsLayout,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
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
