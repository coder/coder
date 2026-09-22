import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { ContextUsageIndicator } from "./ContextUsageIndicator";

const meta: Meta<typeof ContextUsageIndicator> = {
	title: "pages/AgentsPage/ContextUsageIndicator",
	component: ContextUsageIndicator,
	args: {
		onOpenDetails: fn(),
		usage: {
			usedTokens: 26_000,
			contextLimitTokens: 1_100_000,
			compressionThreshold: 70,
		},
	},
};
export default meta;
type Story = StoryObj<typeof ContextUsageIndicator>;
export const Default: Story = {};
export const Open: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: /Context usage/ }),
		);
	},
};
export const Unavailable: Story = { ...Open, args: { usage: null } };
export const Disabled: Story = {
	...Open,
	args: {
		usage: {
			usedTokens: 26_000,
			contextLimitTokens: 1_100_000,
			compressionThreshold: 100,
		},
	},
};
export const Estimated: Story = {
	...Open,
	args: {
		usage: {
			usedTokens: 350,
			contextLimitTokens: 200_000,
			compressionThreshold: 70,
			estimated: true,
		},
	},
};
export const UnknownBudget: Story = {
	...Open,
	args: { usage: { usedTokens: 26_000 } },
};
export const ZeroThreshold: Story = {
	...Open,
	args: {
		usage: {
			usedTokens: 26_000,
			contextLimitTokens: 1_100_000,
			compressionThreshold: 0,
		},
	},
};
export const Narrow: Story = {
	...Estimated,
	parameters: { viewport: { defaultViewport: "mobile1" } },
};
export const KeyboardFocus: Story = {
	play: async () => {
		await userEvent.tab();
		await userEvent.keyboard("{Enter}");
		await userEvent.tab();
	},
};
