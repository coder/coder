import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { organizationRolesQueryKey } from "#/api/queries/roles";
import {
	assignableRole,
	MockDefaultOrganization,
	MockRoleWithOrgPermissions,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withOrganizationSettingsProvider,
	withToaster,
} from "#/testHelpers/storybook";
import CreateEditRolePage from "./CreateEditRolePage";

const organizationName = MockDefaultOrganization.name;
const rolesPath = `/organizations/${organizationName}/roles`;

const meta = {
	title: "pages/OrganizationCreateEditRolePage/CreateEditRolePage",
	component: CreateEditRolePage,
	decorators: [withToaster, withOrganizationSettingsProvider],
	parameters: {
		queries: [
			{
				key: organizationRolesQueryKey(organizationName),
				data: [assignableRole(MockRoleWithOrgPermissions, true)],
			},
		],
		reactRouter: reactRouterParameters({
			location: {
				path: `${rolesPath}/create`,
				pathParams: { organization: organizationName },
			},
			routing: [
				{
					path: "/organizations/:organization/roles",
					element: <div>Roles list</div>,
				},
				{
					path: "/organizations/:organization/roles/create",
					useStoryElement: true,
				},
				{
					path: "/organizations/:organization/roles/:roleName",
					useStoryElement: true,
				},
			],
		}),
	},
} satisfies Meta<typeof CreateEditRolePage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ShowsSuccessNotificationOnSubmit: Story = {
	beforeEach: () => {
		spyOn(API, "createOrganizationRole").mockResolvedValue({
			...MockRoleWithOrgPermissions,
			name: "auditor",
			display_name: "Auditor",
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.type(await canvas.findByLabelText(/^Name/), "auditor");
		await user.click(
			canvas.getByRole("button", { name: "Create custom role" }),
		);

		await waitFor(() => {
			expect(API.createOrganizationRole).toHaveBeenCalled();
		});
		await expect(canvas.findByText("Roles list")).resolves.toBeVisible();
	},
};

export const ShowsErrorWhenCreateFails: Story = {
	beforeEach: () => {
		spyOn(API, "createOrganizationRole").mockRejectedValue(
			mockApiError({ message: "A role named auditor already exists." }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.type(await canvas.findByLabelText(/^Name/), "auditor");
		await user.click(
			canvas.getByRole("button", { name: "Create custom role" }),
		);

		await canvas.findAllByText("A role named auditor already exists.");
	},
};

export const RoleNotFound: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: `${rolesPath}/missing-role`,
				pathParams: {
					organization: organizationName,
					roleName: "missing-role",
				},
			},
			routing: [
				{
					path: "/organizations/:organization/roles",
					element: <div>Roles list</div>,
				},
				{
					path: "/organizations/:organization/roles/create",
					useStoryElement: true,
				},
				{
					path: "/organizations/:organization/roles/:roleName",
					useStoryElement: true,
				},
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Role not found")).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: /back to roles/i }),
		).toHaveAttribute("href", rolesPath);
	},
};

export const NavigatesBackToRoles: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		await user.click(
			await canvas.findByRole("link", { name: /back to roles/i }),
		);
		await expect(await canvas.findByText("Roles list")).toBeVisible();
	},
};
