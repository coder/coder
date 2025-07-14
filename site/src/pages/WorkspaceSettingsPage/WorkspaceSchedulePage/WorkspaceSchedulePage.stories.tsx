import type { Meta, StoryObj } from "@storybook/react";
import { getAuthorizationKey } from "api/queries/authCheck";
import { templateByNameKey } from "api/queries/templates";
import { workspaceByOwnerAndNameKey } from "api/queries/workspaces";
import { AuthProvider } from "contexts/auth/AuthProvider";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { permissionChecks } from "modules/permissions";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import {
	MockAppearanceConfig,
	MockAuthMethodsAll,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockExperiments,
	MockPrebuiltWorkspace,
	MockTemplate,
	MockUserAppearanceSettings,
	MockUserOwner,
	MockWorkspace,
} from "testHelpers/entities";
import WorkspaceSchedulePage from "./WorkspaceSchedulePage";

import { WorkspaceSettingsContext } from "../WorkspaceSettingsLayout";

const meta = {
	title: "pages/WorkspaceSchedulePage",
	component: RequireAuth,
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					username: `@${MockWorkspace.owner_name}`,
					workspace: MockWorkspace.name,
				},
			},
			routing: reactRouterOutlet(
				{
					path: "/:username/:workspace/settings/schedule",
				},
				<WorkspaceSchedulePage />,
			),
		}),
		queries: [
			{ key: ["me"], data: MockUserOwner },
			{ key: ["authMethods"], data: MockAuthMethodsAll },
			{ key: ["hasFirstUser"], data: true },
			{ key: ["buildInfo"], data: MockBuildInfo },
			{ key: ["entitlements"], data: MockEntitlements },
			{ key: ["experiments"], data: MockExperiments },
			{ key: ["appearance"], data: MockAppearanceConfig },
			{ key: ["organizations"], data: [MockDefaultOrganization] },
			{
				key: getAuthorizationKey({ checks: permissionChecks }),
				data: { editWorkspaceProxies: true },
			},
			{ key: ["me", "appearance"], data: MockUserAppearanceSettings },
			{
				key: workspaceByOwnerAndNameKey(
					MockWorkspace.owner_name,
					MockWorkspace.name,
				),
				data: MockWorkspace,
			},
			{
				key: getAuthorizationKey({
					checks: {
						updateWorkspace: {
							object: {
								resource_type: "workspace",
								resource_id: MockWorkspace.id,
								owner_id: MockWorkspace.owner_id,
							},
							action: "update",
						},
					},
				}),
				data: { updateWorkspace: true },
			},
			{
				key: templateByNameKey(
					MockWorkspace.organization_id,
					MockWorkspace.template_name,
				),
				data: MockTemplate,
			},
		],
	},
	decorators: [
		(Story, { parameters }) => {
			const workspace = parameters.workspace || MockWorkspace;
			return (
				<AuthProvider>
					<WorkspaceSettingsContext.Provider value={workspace}>
						<Story />
					</WorkspaceSettingsContext.Provider>
				</AuthProvider>
			);
		},
	],
} satisfies Meta<typeof WorkspaceSchedulePage>;

export default meta;
type Story = StoryObj<typeof WorkspaceSchedulePage>;

export const RegularWorkspace: Story = {
	parameters: {
		workspace: MockWorkspace,
	},
};

export const PrebuiltWorkspace: Story = {
	parameters: {
		workspace: MockPrebuiltWorkspace,
	},
};
