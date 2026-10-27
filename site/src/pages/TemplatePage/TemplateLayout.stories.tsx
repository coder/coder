import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import {
	templateResourceRunLogsKey,
	templateResourcesKey,
} from "#/api/queries/templateResources";
import { templateKey } from "#/api/queries/templates";
import { workspacesKey } from "#/api/queries/workspaces";
import { workspacePermissionChecks } from "#/modules/permissions/workspaces";
import {
	MockTemplate,
	MockTemplateVersion,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import {
	MockTemplateResourceLog,
	MockTemplateResourceRun,
	MockTemplateResources,
} from "#/testHelpers/templateResources";
import { TemplateLayout } from "./TemplateLayout";
import TemplateSharedResourcesPage from "./TemplateSharedResourcesPage/TemplateSharedResourcesPage";

const mockLayoutData = {
	template: MockTemplate,
	activeVersion: MockTemplateVersion,
	permissions: { canUpdateTemplate: true },
};

const meta = {
	title: "pages/TemplatePage/TemplateLayout",
	component: TemplateLayout,
	decorators: [withAuthProvider, withDashboardProvider],
	args: { children: <TemplateSharedResourcesPage /> },
	beforeEach: () => {
		// Resource polling overrides the Storybook QueryClient's interval default.
		const resources = spyOn(API, "getTemplateResources").mockResolvedValue(
			MockTemplateResources,
		);
		const logs = spyOn(API, "getTemplateResourceRunLogs").mockResolvedValue([
			MockTemplateResourceLog,
		]);
		return () => {
			resources.mockRestore();
			logs.mockRestore();
		};
	},
	parameters: {
		user: MockUserOwner,
		experiments: ["template-resources"],
		reactRouter: reactRouterParameters({
			location: { pathParams: { template: MockTemplate.name } },
			routing: { path: "/templates/:template/resources" },
		}),
	},
	play: async ({ canvasElement }) => {
		// Wait for auth before capturing the tabs and nested resource page.
		await within(canvasElement).findByRole("link", {
			name: "Per-workspace resources",
		});
	},
} satisfies Meta<typeof TemplateLayout>;
export default meta;
type Story = StoryObj<typeof meta>;

export const SharedResourcesEnabled: Story = {
	parameters: {
		layout: "fullscreen",
		pixel: { matrix: pixelWithPhone },
		queries: [
			{ key: templateKey(MockTemplate.name), data: mockLayoutData },
			{
				key: getAuthorizationKey({
					checks: workspacePermissionChecks(
						MockTemplate.organization_id,
						MockUserOwner.id,
					),
				}),
				data: { createWorkspaceForUserID: true },
			},
			{
				key: workspacesKey({
					q: `organization:${MockTemplate.organization_name} template:${MockTemplate.name}`,
				}),
				data: { workspaces: [], count: 0 },
			},
			{
				key: templateResourcesKey(MockTemplate.id),
				data: MockTemplateResources,
			},
			{
				key: templateResourceRunLogsKey(
					MockTemplate.id,
					MockTemplateResourceRun.id,
				),
				data: [MockTemplateResourceLog],
			},
		],
	},
};
export const ExperimentDisabled: Story = {
	parameters: {
		experiments: [],
		queries: [
			{ key: templateKey(MockTemplate.name), data: mockLayoutData },
			{
				key: getAuthorizationKey({
					checks: workspacePermissionChecks(
						MockTemplate.organization_id,
						MockUserOwner.id,
					),
				}),
				data: { createWorkspaceForUserID: true },
			},
			{
				key: workspacesKey({
					q: `organization:${MockTemplate.organization_name} template:${MockTemplate.name}`,
				}),
				data: { workspaces: [], count: 0 },
			},
		],
	},
};
export const PermissionDenied: Story = {
	parameters: {
		user: MockUserMember,
		queries: [
			{
				key: templateKey(MockTemplate.name),
				data: { ...mockLayoutData, permissions: { canUpdateTemplate: false } },
			},
			{
				key: getAuthorizationKey({
					checks: workspacePermissionChecks(
						MockTemplate.organization_id,
						MockUserMember.id,
					),
				}),
				data: { createWorkspaceForUserID: true },
			},
		],
	},
};
