import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { AnnouncementBannerDialog } from "./AnnouncementBannerDialog";

const meta: Meta<typeof AnnouncementBannerDialog> = {
	title: "pages/DeploymentSettingsPage/AnnouncementBannerDialog",
	component: AnnouncementBannerDialog,
	args: {
		banner: {
			enabled: true,
			message: "The beep-bop will be boop-beeped on Saturday at 12AM PST.",
			background_color: "#ffaff3",
		},
		onCancel: fn(),
		onUpdate: fn(async () => undefined),
	},
};

export default meta;
type Story = StoryObj<typeof AnnouncementBannerDialog>;

const Example: Story = {};

export { Example as AnnouncementBannerDialog };

export const EditsMessage: Story = {
	play: async () => {
		const body = within(document.body);
		const message = await body.findByLabelText("Message");
		await userEvent.clear(message);
		await userEvent.type(message, "Scheduled maintenance tonight.");
	},
};
