import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
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
	play: async ({ args }) => {
		const body = within(document.body);
		const message = await body.findByLabelText("Message");
		await expect(message).toHaveValue(
			"The beep-bop will be boop-beeped on Saturday at 12AM PST.",
		);

		await userEvent.clear(message);
		await userEvent.type(message, "Scheduled maintenance tonight.");
		await userEvent.click(body.getByRole("button", { name: "Update" }));
		await expect(args.onUpdate).toHaveBeenCalledWith(
			expect.objectContaining({ message: "Scheduled maintenance tonight." }),
		);
	},
};

export const CancelClosesDialog: Story = {
	play: async ({ args }) => {
		const body = within(document.body);
		await userEvent.click(await body.findByRole("button", { name: "Cancel" }));
		await expect(args.onCancel).toHaveBeenCalled();
	},
};
