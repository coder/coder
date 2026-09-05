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
			expect(tooltip).toHaveTextContent("Total requests");
			expect(tooltip).toHaveTextContent("Blocked");
		});
	},
};

// Hovering the disabled indicator's info button reveals why tracking is off.
export const DisabledHover: Story = {
	args: {
		summary: undefined,
	},
	play: async () => {
		await userEvent.hover(
			screen.getByRole("button", {
				name: "Why network request tracking is disabled",
			}),
		);
		await waitFor(() =>
			expect(screen.getByRole("tooltip")).toHaveTextContent(
				"Agent Firewall is off. Enable it in the workspace template to track network requests.",
			),
		);
	},
};
