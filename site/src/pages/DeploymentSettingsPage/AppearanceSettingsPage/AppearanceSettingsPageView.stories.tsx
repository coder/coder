import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
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
			hide_codernauts: false,
		},
		isEntitled: false,
		canViewPremium: MockPermissions.viewAllLicenses,
		onSaveAppearance: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AppearanceSettingsPageView>;

export const Entitled: Story = {
	args: {
		isEntitled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("form", { name: "Appearance settings" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("heading", { name: "Announcement Banners" }),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
	},
};

export const NotEntitled: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const cta = canvas.getByRole("link", { name: "Start trial for free" });
		await expect(cta).toHaveAttribute("href", "/deployment/premium");
		await expect(
			canvas.queryByRole("form", { name: "Appearance settings" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("heading", { name: "Announcement Banners" }),
		).not.toBeInTheDocument();
	},
};

export const NotEntitledWithoutLicenseAccess: Story = {
	args: {
		canViewPremium: false,
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

export const HideCodernautsToggle: Story = {
	args: {
		isEntitled: true,
	},
	play: async ({ canvasElement, args, step }) => {
		const canvas = within(canvasElement);
		await step("toggling the switch saves the setting", async () => {
			await userEvent.click(
				canvas.getByRole("switch", { name: "Hide Codernauts game" }),
			);
			await waitFor(() =>
				expect(args.onSaveAppearance).toHaveBeenCalledWith({
					hide_codernauts: true,
				}),
			);
		});
	},
};

export const HideCodernautsNotEntitled: Story = {
	play: async ({ canvasElement, args, step }) => {
		const canvas = within(canvasElement);
		await step("the switch saves even without entitlement", async () => {
			const switchEl = canvas.getByRole("switch", {
				name: "Hide Codernauts game",
			});
			expect(switchEl).toBeEnabled();
			await userEvent.click(switchEl);
			await waitFor(() =>
				expect(args.onSaveAppearance).toHaveBeenCalledWith({
					hide_codernauts: true,
				}),
			);
		});
	},
};
