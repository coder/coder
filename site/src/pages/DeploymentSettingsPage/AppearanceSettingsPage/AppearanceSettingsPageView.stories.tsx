import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
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
	},
};

export default meta;
type Story = StoryObj<typeof AppearanceSettingsPageView>;

/** The badge is passive: hovering it must not surface a paywall. */
const expectPassiveBadge = async (label: string) => {
	await userEvent.hover(screen.getByText(label));
	await expect(
		screen.queryByRole("link", { name: "Read the documentation" }),
	).not.toBeInTheDocument();
};

export const Entitled: Story = {
	args: {
		isEntitled: true,
	},
	play: async () => {
		await expectPassiveBadge("Enterprise");
	},
};

export const NotEntitled: Story = {
	play: async () => {
		await expectPassiveBadge("Premium");
	},
};
