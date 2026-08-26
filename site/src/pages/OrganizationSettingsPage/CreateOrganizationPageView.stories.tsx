import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { MockPermissions, mockApiError } from "#/testHelpers/entities";
import { docs } from "#/utils/docs";
import { CreateOrganizationPageView } from "./CreateOrganizationPageView";

const meta: Meta<typeof CreateOrganizationPageView> = {
	title: "pages/CreateOrganizationPageView",
	component: CreateOrganizationPageView,
	args: {
		isEntitled: true,
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof CreateOrganizationPageView>;

export const Example: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The badge is passive: hovering it must not surface a paywall.
		await userEvent.hover(canvas.getByText("Premium"));
		await expect(
			screen.queryByRole("link", { name: "Learn more about premium" }),
		).not.toBeInTheDocument();
	},
};

export const NotEntitled: Story = {
	args: {
		isEntitled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("link", { name: /View docs/ }),
		).toHaveAttribute("href", docs("/admin/users/organizations"));
		const cta = canvas.getByRole("link", { name: "Start trial for free" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
	},
};

export const NotEntitledWithoutLicenseAccess: Story = {
	args: {
		isEntitled: false,
		permissions: { ...MockPermissions, viewAllLicenses: false },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
	},
};

export const WithError: Story = {
	args: { error: "Oh no!" },
};

export const InvalidName: Story = {
	args: {
		error: mockApiError({
			message: "Display name is bad",
			validations: [
				{
					field: "display_name",
					detail: "That display name is terrible. What were you thinking?",
				},
			],
		}),
	},
};
