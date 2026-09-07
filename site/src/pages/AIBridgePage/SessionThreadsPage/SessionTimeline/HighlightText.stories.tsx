import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { HighlightText } from "./HighlightText";

const meta: Meta<typeof HighlightText> = {
	title: "pages/AIBridgePage/SessionTimeline/HighlightText",
	component: HighlightText,
	args: {
		text: "Relay through the relay.",
		query: "relay",
	},
};

export default meta;
type Story = StoryObj<typeof HighlightText>;

export const Matches: Story = {
	play: async ({ canvas }) => {
		await expect(canvas.getAllByText(/^relay$/i)).toHaveLength(2);
	},
};

export const NoMatch: Story = {
	args: {
		query: "missing",
	},
	play: async ({ canvas }) => {
		await expect(canvas.getByText("Relay through the relay.")).toBeVisible();
	},
};
