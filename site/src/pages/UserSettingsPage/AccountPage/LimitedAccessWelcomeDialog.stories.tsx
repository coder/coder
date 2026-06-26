import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { LimitedAccessWelcomeDialog } from "./LimitedAccessWelcomeDialog";

// The dialog persists dismissal in localStorage keyed by userID, so each
// story uses a different ID to start with the dialog open.
const meta: Meta<typeof LimitedAccessWelcomeDialog> = {
	title: "pages/UserSettingsPage/LimitedAccessWelcomeDialog",
	component: LimitedAccessWelcomeDialog,
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({ location: { pathParams: {} } }),
	},
	beforeEach: () => {
		try {
			localStorage.clear();
		} catch {
			// noop
		}
	},
};

export default meta;
type Story = StoryObj<typeof LimitedAccessWelcomeDialog>;

export const Default: Story = {
	args: { userID: "story-default-user" },
};

export const Dismiss: Story = {
	args: { userID: "story-dismiss-user" },
	play: async ({ canvasElement, step }) => {
		const screen = within(canvasElement.ownerDocument.body);
		await step("dialog is visible on first visit", async () => {
			await expect(
				await screen.findByText("You\u2019re signed in as a Gateway Account"),
			).toBeVisible();
		});
		await step("user can dismiss with the close button", async () => {
			await userEvent.click(screen.getByRole("button", { name: /close/i }));
			await expect(
				screen.queryByText("You\u2019re signed in as a Gateway Account"),
			).not.toBeInTheDocument();
		});
	},
};
