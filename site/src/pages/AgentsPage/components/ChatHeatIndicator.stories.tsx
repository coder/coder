import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import type { ChatHeat } from "./ChatConversation/chatHeat";
import { ChatHeatIndicator } from "./ChatHeatIndicator";

const heatWith = (lastPromptTokens: number): ChatHeat => ({
	lastPromptTokens,
	lastRequestAt: "2026-01-01T00:00:00Z",
	lastModelConfigId: "model-a",
	boundary: undefined,
	lastTurn: {
		requestCount: 3,
		missedTokens: 3_400,
		reusableTokens: lastPromptTokens - 1_200,
		isPartial: false,
	},
});

const MINUTE_MS = 60_000;

const meta: Meta<typeof ChatHeatIndicator> = {
	title: "pages/AgentsPage/ChatHeatIndicator",
	component: ChatHeatIndicator,
	args: {
		heat: heatWith(165_000),
		remainingMs: 4 * MINUTE_MS,
		isModelChanged: false,
		canSwitchModelBack: true,
	},
};

export default meta;
type Story = StoryObj<typeof ChatHeatIndicator>;

// Opens the tooltip and waits for it so the screenshot captures the
// breakdown.
const hoverTrigger: Story["play"] = async ({ canvasElement }) => {
	await userEvent.hover(
		within(canvasElement).getByRole("button", { name: /next message/i }),
	);
	await screen.findByRole("tooltip");
};

/** Warm cache with four minutes left; the grey segment shows the cold reading. */
export const Warm: Story = { play: hoverTrigger };

export const WarmLargeContext: Story = {
	args: { heat: heatWith(600_000) },
	play: hoverTrigger,
};

export const LastMinute: Story = {
	args: { remainingMs: 42_000 },
	play: hoverTrigger,
};

export const ExpiredLow: Story = {
	args: { heat: heatWith(50_000), remainingMs: -12 * MINUTE_MS },
	play: hoverTrigger,
};

export const ExpiredHigh: Story = {
	args: { remainingMs: -12 * MINUTE_MS },
	play: hoverTrigger,
};

export const ExpiredSaturated: Story = {
	args: { heat: heatWith(300_000), remainingMs: -3 * 60 * MINUTE_MS },
	play: hoverTrigger,
};

export const ModelChanged: Story = {
	args: { isModelChanged: true },
	play: hoverTrigger,
};

export const Generating: Story = {
	args: { remainingMs: undefined },
	play: hoverTrigger,
};

export const AfterCompaction: Story = {
	args: {
		heat: {
			lastPromptTokens: 0,
			lastRequestAt: "",
			lastModelConfigId: undefined,
			boundary: "compacted",
			lastTurn: undefined,
		},
		remainingMs: undefined,
	},
	play: hoverTrigger,
};

export const PartialTurn: Story = {
	args: {
		heat: {
			...heatWith(165_000),
			lastTurn: {
				requestCount: 12,
				missedTokens: 0,
				reusableTokens: 0,
				isPartial: true,
			},
		},
	},
	play: hoverTrigger,
};

export const IconOnlyWarm: Story = {};

export const IconOnlyLastMinute: Story = {
	args: { remainingMs: 42_000 },
};

export const IconOnlyExpired: Story = {
	args: { remainingMs: -MINUTE_MS },
};

export const IconOnlyModelChanged: Story = {
	args: { isModelChanged: true },
};

export const Mobile: Story = {
	args: { remainingMs: -12 * MINUTE_MS },
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
	// Opens the popover so the screenshot captures the mobile layout.
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: /next message/i }),
		);
	},
};
