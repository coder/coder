import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { MockPermissions } from "#/testHelpers/entities";
import { AppearanceSettingsPageView } from "./AppearanceSettingsPageView";

const meta: Meta<typeof AppearanceSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/AppearanceSettingsPageView",
	component: AppearanceSettingsPageView,
	args: {
		appearance: {
			application_name: "Foobar",
			logo_url: "https://github.com/coder.png",
			service_banner: {
				enabled: false,
				message: "",
				background_color: "#00ff00",
			},
			announcement_banners: [
				{
					enabled: true,
					message: "The beep-bop will be boop-beeped on Saturday at 12AM PST.",
					background_color: "#ffaff3",
				},
			],
		},
		isEntitled: false,
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof AppearanceSettingsPageView>;

export const Entitled: Story = {
	args: {
		isEntitled: true,
	},
	play: async () => {
		// The badge is passive: hovering it must not surface a paywall.
		await userEvent.hover(screen.getByText("Enterprise"));
		await expect(
			screen.queryByRole("link", { name: "Read the documentation" }),
		).not.toBeInTheDocument();
	},
};

export const NotEntitled: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const cta = canvas.getByRole("link", { name: "Learn about Premium" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
	},
};

export const NotEntitledWithoutLicenseAccess: Story = {
	args: {
		permissions: { ...MockPermissions, viewAllLicenses: false },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Learn about Premium" }),
		).not.toBeInTheDocument();
	},
};
