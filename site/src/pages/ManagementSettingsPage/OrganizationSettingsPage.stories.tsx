import type { Meta, StoryObj } from "@storybook/react";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockDefaultOrganization,
	MockOrganization,
	MockOrganization2,
	MockUser,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withManagementSettingsProvider,
} from "testHelpers/storybook";
import OrganizationSettingsPage from "./OrganizationSettingsPage";

const meta: Meta<typeof OrganizationSettingsPage> = {
	title: "pages/OrganizationSettingsPage",
	component: OrganizationSettingsPage,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		withManagementSettingsProvider,
	],
	parameters: {
		showOrganizations: true,
		user: MockUser,
		features: ["multiple_organizations"],
		permissions: { viewDeploymentValues: true },
		queries: [
			{
				key: ["organizations", [MockDefaultOrganization.id], "permissions"],
				data: {},
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationSettingsPage>;

export const NoRedirectableOrganizations: Story = {};

export const OrganizationDoesNotExist: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { pathParams: { organization: "does-not-exist" } },
			routing: { path: "/organizations/:organization" },
		}),
	},
};

export const CannotEditOrganization: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { pathParams: { organization: MockDefaultOrganization.name } },
			routing: { path: "/organizations/:organization" },
		}),
	},
};

export const CanEditOrganization: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { pathParams: { organization: MockDefaultOrganization.name } },
			routing: { path: "/organizations/:organization" },
		}),
		queries: [
			{
				key: ["organizations", [MockDefaultOrganization.id], "permissions"],
				data: {
					[MockDefaultOrganization.id]: {
						editOrganization: true,
					},
				},
			},
		],
	},
};

export const CanEditOrganizationNotEntitled: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { pathParams: { organization: MockDefaultOrganization.name } },
			routing: { path: "/organizations/:organization" },
		}),
		features: [],
		queries: [
			{
				key: ["organizations", [MockDefaultOrganization.id], "permissions"],
				data: {
					[MockDefaultOrganization.id]: {
						editOrganization: true,
					},
				},
			},
		],
	},
};
