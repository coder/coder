import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { NetworkCallBadges } from "./NetworkCallBadges";

const meta: Meta<typeof NetworkCallBadges> = {
	title: "pages/AIBridgePage/NetworkCallBadges",
	component: NetworkCallBadges,
};

export default meta;
type Story = StoryObj<typeof NetworkCallBadges>;

export const TotalAndBlocked: Story = {
	args: {
		summary: { total: 23, blocked: 2 },
	},
};

export const NoBlocked: Story = {
	args: {
		summary: { total: 23, blocked: 0 },
	},
};

export const NoActivity: Story = {
	args: {
		summary: { total: 0, blocked: 0 },
	},
};

export const Disabled: Story = {
	args: {
		summary: undefined,
	},
};

export const LargeCounts: Story = {
	args: {
		summary: { total: 12_480, blocked: 320 },
	},
};

// Tabbing to the badges reveals the breakdown tooltip without a mouse.
export const TotalAndBlockedKeyboard: Story = {
	args: {
		summary: { total: 23, blocked: 2 },
	},
	play: async () => {
		await userEvent.tab();
		await waitFor(() => {
			const tooltip = screen.getByRole("tooltip");
			expect(tooltip).toHaveTextContent("Total calls");
			expect(tooltip).toHaveTextContent("Blocked");
		});
	},
};

// Tabbing to the disabled indicator's info button and pressing Enter reveals
// the reason popover without a mouse.
export const DisabledKeyboard: Story = {
	args: {
		summary: undefined,
	},
	play: async () => {
		await userEvent.tab();
		await userEvent.keyboard("{Enter}");
		await waitFor(() =>
			expect(screen.getByRole("dialog")).toHaveTextContent(
				"Network call monitoring was not active for this session.",
			),
		);
	},
};
