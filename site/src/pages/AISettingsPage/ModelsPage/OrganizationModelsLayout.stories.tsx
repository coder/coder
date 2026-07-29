import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import OrganizationModelsLayout from "./OrganizationModelsLayout";

const meta: Meta<typeof OrganizationModelsLayout> = {
	title: "pages/AISettingsPage/OrganizationModelsLayout",
	component: OrganizationModelsLayout,
	decorators: [withDashboardProvider],
	parameters: {
		showOrganizations: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/organizations/${MockDefaultOrganization.name}/models`,
			},
			routing: [
				{
					path: "/ai/settings/organizations/:organization/models",
					useStoryElement: true,
				},
			],
		}),
		queries: [
			{
				key: ["organizations", "permissions"],
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
					[MockOrganization2.id]: MockOrganizationPermissions,
				},
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationModelsLayout>;

// Switching the autocomplete navigates to the selected organization's
// models page.
export const SwitchOrganizationNavigates: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByTestId("organization-autocomplete"),
		);
		await waitFor(() => {
			expect(
				screen.getByText(MockOrganization2.display_name),
			).toBeInTheDocument();
		});
		await userEvent.click(screen.getByText(MockOrganization2.display_name));
		await waitFor(() => {
			expect(window.location.pathname).toBe(
				`/ai/settings/organizations/${MockOrganization2.name}/models`,
			);
		});
	},
};
