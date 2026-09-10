import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { IdpUnseenClaimWarning } from "./IdpUnseenClaimWarning";

const meta: Meta<typeof IdpUnseenClaimWarning> = {
	title: "modules/idpSync/IdpUnseenClaimWarning",
	component: IdpUnseenClaimWarning,
};

export default meta;
type Story = StoryObj<typeof IdpUnseenClaimWarning>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(
			canvas.getByRole("button", { name: "Unknown claim value" }),
		);
		await screen.findByRole("tooltip", {
			name: /has not be seen in the specified claim field/i,
		});
	},
};
