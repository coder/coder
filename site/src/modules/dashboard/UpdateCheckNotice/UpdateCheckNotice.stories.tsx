import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor } from "storybook/test";
import { Toaster } from "#/components/Toaster/Toaster";
import { UpdateCheckNotice } from "./UpdateCheckNotice";

const meta: Meta<typeof UpdateCheckNotice> = {
	title: "modules/dashboard/UpdateCheckNotice",
	component: UpdateCheckNotice,
	// Render the Toaster before the story so it subscribes to sonner before the
	// notice emits its toast from a mount effect; earlier emits are dropped.
	decorators: [
		(Story) => (
			<>
				<Toaster />
				<Story />
			</>
		),
	],
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
	play: async ({ args }) => {
		// The toast renders in a portal on document.body, so query the screen.
		await expect(
			await screen.findByText(/Coder v0\.12\.9 is now available/),
		).toBeInTheDocument();
		await expect(
			screen.getByRole("link", { name: "Release notes" }),
		).toHaveAttribute(
			"href",
			"https://github.com/coder/coder/releases/tag/v0.12.9",
		);
		await expect(
			screen.getByRole("link", { name: "Upgrade instructions" }),
		).toBeInTheDocument();

		await userEvent.click(screen.getByRole("button", { name: "Close toast" }));
		await waitFor(() => expect(args.onDismiss).toHaveBeenCalled());
	},
};

export const WithoutDeploymentBanner: Story = {
	args: {
		aboveDeploymentBanner: false,
	},
};
