import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
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
		onSaveAppearance: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AppearanceSettingsPageView>;

export const Entitled: Story = {
	args: {
		isEntitled: true,
	},
};

export const NotEntitled: Story = {};

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
