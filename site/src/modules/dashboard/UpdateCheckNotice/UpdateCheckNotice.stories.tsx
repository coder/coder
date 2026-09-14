import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { UpdateCheckNotice } from "./UpdateCheckNotice";

const meta: Meta<typeof UpdateCheckNotice> = {
	title: "modules/dashboard/UpdateCheckNotice",
	component: UpdateCheckNotice,
	args: {
		version: "v0.12.9",
		releaseNotesUrl: "https://github.com/coder/coder/releases/tag/v0.12.9",
		onDismiss: fn(),
		aboveDeploymentBanner: true,
	},
	parameters: {
		layout: "fullscreen",
	},
};

export default meta;
type Story = StoryObj<typeof UpdateCheckNotice>;

export const AboveDeploymentBanner: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Coder v0\.12\.9 is now available/),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "release notes" }),
		).toHaveAttribute(
			"href",
			"https://github.com/coder/coder/releases/tag/v0.12.9",
		);
		await expect(
			canvas.getByRole("link", { name: "upgrade instructions" }),
		).toBeVisible();

		await userEvent.click(canvas.getByRole("button", { name: "Dismiss" }));
		await expect(args.onDismiss).toHaveBeenCalled();
	},
};

export const WithoutDeploymentBanner: Story = {
	args: {
		aboveDeploymentBanner: false,
	},
};
