import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { ChatHeat } from "./ChatConversation/chatHeat";
import { ChatHeatIndicator } from "./ChatHeatIndicator";

const baseHeat: ChatHeat = {
	heat: 0.05,
	label: "cool",
	missRate: 0.04,
	lastTurnRequestCount: 3,
	lastTurnFreshTokens: 1_200,
	lastTurnCacheReadTokens: 84_000,
	lastTurnIsFirst: false,
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
		within(canvasElement).getByRole("button", { name: /chat heat/i }),
	);
};

export const Cool: Story = { play: hoverTrigger };

export const Warm: Story = {
	args: {
		heat: {
			...baseHeat,
			heat: 0.49,
			label: "warm",
			missRate: 0.35,
			lastTurnRequestCount: 3,
			lastTurnFreshTokens: 42_000,
			lastTurnCacheReadTokens: 78_000,
			lastPromptTokens: 120_000,
		},
	},
	play: hoverTrigger,
};

export const Hot: Story = {
	args: {
		heat: {
			...baseHeat,
			heat: 0.95,
			label: "hot",
			missRate: 0.97,
			lastTurnRequestCount: 3,
			lastTurnFreshTokens: 118_000,
			lastTurnCacheReadTokens: 3_000,
			lastPromptTokens: 121_000,
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
			lastTurnRequestCount: 1,
			lastTurnFreshTokens: 90_000,
			lastTurnCacheReadTokens: 0,
			lastTurnIsFirst: true,
		},
	},
	play: hoverTrigger,
};

export const IconOnly: Story = {
	args: {
		heat: { ...baseHeat, heat: 0.7, label: "hot" },
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
			within(canvasElement).getByRole("button", { name: /chat heat/i }),
		);
	},
};

export const IconOnlyCoolExpired: Story = {
	args: {
		isCacheExpired: true,
	},
};
