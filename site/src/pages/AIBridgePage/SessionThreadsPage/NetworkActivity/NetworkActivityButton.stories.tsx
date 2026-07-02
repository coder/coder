import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { mockNetworkActivity } from "./mocks";
import { NetworkActivityButton } from "./NetworkActivityButton";

const meta: Meta<typeof NetworkActivityButton> = {
	title: "pages/AIBridgePage/NetworkActivity/Button",
	component: NetworkActivityButton,
	parameters: {
		// Keep the dialog room below the trigger in Storybook's iframe.
		layout: "padded",
	},
};

export default meta;
type Story = StoryObj<typeof NetworkActivityButton>;

const open = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
	const canvas = within(canvasElement);
	const trigger = await canvas.findByRole("button", {
		name: /Network activity/i,
	});
	await userEvent.click(trigger);
	// Popover content is portaled, so look at the whole document.
	const body = within(document.body);
	await waitFor(() =>
		expect(
			body.getByRole("heading", { name: /Network activity/i }),
		).toBeVisible(),
	);
};

export const AllAllowed: Story = {
	args: { networkActivity: mockNetworkActivity("all-allowed") },
	play: open,
};

export const Mixed: Story = {
	args: { networkActivity: mockNetworkActivity("mixed") },
	play: open,
};

export const ErrorOnly: Story = {
	args: { networkActivity: mockNetworkActivity("error-only") },
	play: open,
};

export const MidSessionFailure: Story = {
	args: { networkActivity: mockNetworkActivity("mid-session-failure") },
	play: open,
};

export const Many: Story = {
	args: { networkActivity: mockNetworkActivity("many") },
	play: open,
};

// With no events the button does not render. Verifies the empty-state.
export const HiddenWhenEmpty: Story = {
	args: { networkActivity: mockNetworkActivity("none") },
};
