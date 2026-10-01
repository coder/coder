import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { ChatHeat } from "./ChatConversation/chatHeat";
import { ChatHeatIndicator } from "./ChatHeatIndicator";

const baseHeat: ChatHeat = {
	heat: 0.05,
	label: "low",
	missRate: 0.04,
	lastTurnRequestCount: 3,
	lastTurnMissedTokens: 3_400,
	lastTurnReusableTokens: 84_000,
	lastTurnHasSegmentStart: false,
	lastPromptTokens: 85_200,
	lastRequestAt: "2026-01-01T00:00:00Z",
};

const meta: Meta<typeof ChatHeatIndicator> = {
	title: "pages/AgentsPage/ChatHeatIndicator",
	component: ChatHeatIndicator,
	args: {
		heat: baseHeat,
		isCacheExpired: false,
	},
};

export default meta;
type Story = StoryObj<typeof ChatHeatIndicator>;

// Opens the tooltip so the screenshot captures the breakdown.
const hoverTrigger: Story["play"] = async ({ canvasElement }) => {
	await userEvent.hover(
		within(canvasElement).getByRole("button", { name: /cache misses/i }),
	);
};

export const Low: Story = { play: hoverTrigger };

export const Moderate: Story = {
	args: {
		heat: {
			...baseHeat,
			heat: 0.49,
			label: "moderate",
			missRate: 0.35,
			lastTurnMissedTokens: 42_000,
			lastTurnReusableTokens: 120_000,
			lastPromptTokens: 122_000,
		},
	},
	play: hoverTrigger,
};

export const High: Story = {
	args: {
		heat: {
			...baseHeat,
			heat: 0.95,
			label: "high",
			missRate: 0.97,
			lastTurnMissedTokens: 116_000,
			lastTurnReusableTokens: 120_000,
			lastPromptTokens: 122_000,
		},
	},
	play: hoverTrigger,
};

export const CacheExpired: Story = {
	args: {
		isCacheExpired: true,
	},
	play: hoverTrigger,
};

export const FirstTurn: Story = {
	args: {
		heat: {
			...baseHeat,
			heat: 0,
			missRate: 0,
			lastTurnRequestCount: 1,
			lastTurnMissedTokens: 0,
			lastTurnReusableTokens: 0,
			lastTurnHasSegmentStart: true,
			lastPromptTokens: 90_000,
		},
	},
	play: hoverTrigger,
};

export const IconOnlyHighExpired: Story = {
	args: {
		heat: { ...baseHeat, heat: 0.7, label: "high" },
		isCacheExpired: true,
	},
};

export const Mobile: Story = {
	args: {
		isCacheExpired: true,
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
	// Opens the popover so the screenshot captures the mobile layout.
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: /cache misses/i }),
		);
	},
};

export const IconOnlyLowExpired: Story = {
	args: {
		isCacheExpired: true,
	},
};
