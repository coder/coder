import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { OrganizationModelsSwitcher } from "./OrganizationModelsSwitcher";

const organizations = [MockDefaultOrganization, MockOrganization2];

const meta = {
	title: "pages/AISettingsPage/ModelsPage/OrganizationModelsSwitcher",
	component: OrganizationModelsSwitcher,
	args: {
		activeOrganization: MockDefaultOrganization,
		organizations,
	},
	parameters: {
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
	},
} satisfies Meta<typeof OrganizationModelsSwitcher>;

export default meta;
type Story = StoryObj<typeof OrganizationModelsSwitcher>;

export const Closed: Story = {};

export const SwitchOrganization: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await userEvent.click(
			canvas.getByRole("button", { name: /My Organization/i }),
		);
		const listbox = await body.findByRole("listbox");
		await waitFor(() => {
			expect(listbox).toBeVisible();
		});

		await userEvent.click(
			await body.findByRole("option", { name: /My Organization 2/i }),
		);
		// Selection closes the popover and navigates; the story router
		// keeps the same element mounted under the :organization param.
		await waitFor(() => {
			expect(body.queryByRole("listbox")).not.toBeInTheDocument();
		});
	},
};
