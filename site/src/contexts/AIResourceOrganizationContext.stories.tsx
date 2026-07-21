import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { AIResourceOrganizationSelector } from "#/components/AIResourceOrganizationSelector/AIResourceOrganizationSelector";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { AIResourceOrganizationProvider } from "./AIResourceOrganizationContext";

const OrganizationSelection = () => (
	<AIResourceOrganizationProvider
		isOrganizationPermitted={(permissions) => permissions.createChat}
	>
		<AIResourceOrganizationSelector />
		<div>Organization-owned content</div>
	</AIResourceOrganizationProvider>
);

const allPermissions = (allowed: boolean) =>
	Object.fromEntries(
		[MockDefaultOrganization, MockOrganization2].flatMap((organization) =>
			[
				"createChat",
				"createModel",
				"viewModels",
				"editModels",
				"deleteModels",
				"createMCPServers",
				"viewMCPServers",
				"editMCPServers",
				"deleteMCPServers",
			].map((permission) => [`${organization.id}.${permission}`, allowed]),
		),
	);

const meta: Meta<typeof OrganizationSelection> = {
	title: "contexts/AIResourceOrganizationContext",
	component: OrganizationSelection,
	decorators: [withDashboardProvider],
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		showOrganizations: true,
	},
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockResolvedValue(allPermissions(true));
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationSelection>;

export const SelectsURLOrganization: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents",
				searchParams: {
					organization: MockOrganization2.name,
					tab: "recent",
				},
			},
			routing: [{ path: "/agents", useStoryElement: true }],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.findByRole("button", {
				name: `Organization: ${MockOrganization2.display_name}`,
			}),
		).resolves.toBeVisible();
		await expect(
			canvas.findByText("Organization-owned content"),
		).resolves.toBeVisible();
	},
};

export const SelectsPermittedOrganizationForAction: Story = {
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockResolvedValue({
			...allPermissions(true),
			[`${MockDefaultOrganization.id}.createChat`]: false,
		});
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents",
				searchParams: {
					organization: MockDefaultOrganization.name,
				},
			},
			routing: [{ path: "/agents", useStoryElement: true }],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.findByText("Organization-owned content"),
		).resolves.toBeVisible();
		await expect(
			canvas.queryByRole("button", {
				name: `Organization: ${MockDefaultOrganization.display_name}`,
			}),
		).not.toBeInTheDocument();
	},
};

export const NoOrganizations: Story = {
	parameters: {
		organizations: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.findByText("No permitted organizations found"),
		).resolves.toBeVisible();
	},
};

export const NoPermittedOrganizations: Story = {
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockResolvedValue(allPermissions(false));
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.findByText("No permitted organizations found"),
		).resolves.toBeVisible();
	},
};
