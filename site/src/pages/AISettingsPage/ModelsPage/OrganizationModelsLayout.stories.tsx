import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocation } from "react-router";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import OrganizationModelsLayout from "./OrganizationModelsLayout";

// Surfaces the active route's pathname so the play function can assert
// where the autocomplete's navigation landed.
const PathnameProbe = () => {
	const location = useLocation();
	return <div data-testid="pathname-probe">{location.pathname}</div>;
};

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
				// Production key shape: organizationsPermissions sorts the
				// organization IDs into the middle segment.
				key: [
					"organizations",
					[MockDefaultOrganization.id, MockOrganization2.id].sort(),
					"permissions",
				],
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
	render: () => (
		<>
			<OrganizationModelsLayout />
			<PathnameProbe />
		</>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByTestId("organization-autocomplete"),
		);
		// The active organization sorts first, then case-insensitive
		// alphabetical; MockOrganization2 is the other entry.
		const option = await screen.findByRole("option", {
			name: new RegExp(MockOrganization2.display_name),
		});
		await userEvent.click(option);
		await waitFor(() => {
			expect(screen.getByTestId("pathname-probe")).toHaveTextContent(
				`/ai/settings/organizations/${MockOrganization2.name}/models`,
			);
		});
	},
};
