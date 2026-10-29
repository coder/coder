import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import type { ResolvedCompactionThreshold } from "../utils/compactionTriggers";
import { ContextUsageIndicator } from "./ContextUsageIndicator";

const modelThreshold = (
	percent: number,
	organizationOverrideNotLoaded = false,
): ResolvedCompactionThreshold => ({
	percent,
	source: "model",
	organizationOverrideNotLoaded,
});

const meta: Meta<typeof ContextUsageIndicator> = {
	title: "pages/AgentsPage/ContextUsageIndicator",
	component: ContextUsageIndicator,
	args: {
		onOpenDetails: fn(),
		usage: {
			usedTokens: 211_400,
			contextLimitTokens: 1_000_000,
			compactionThreshold: modelThreshold(70),
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
			compactionThreshold: modelThreshold(100),
		},
	},
};
export const OrganizationOverride: Story = {
	...Open,
	args: {
		usage: {
			usedTokens: 64_000,
			contextLimitTokens: 200_000,
			compactionThreshold: { source: "organization", pointTokens: 32_000 },
		},
	},
};
export const OrganizationOverrideBeyondWindow: Story = {
	...Open,
	args: {
		usage: {
			usedTokens: 5_200,
			contextLimitTokens: 10_000,
			compactionThreshold: { source: "organization", pointTokens: 16_000 },
		},
	},
};
export const OrganizationOverrideNotLoaded: Story = {
	...Open,
	args: {
		usage: {
			usedTokens: 64_000,
			contextLimitTokens: 128_000,
			compactionThreshold: modelThreshold(70, true),
		},
	},
};
export const DisabledOrganizationOverrideNotLoaded: Story = {
	...Open,
	args: {
		usage: {
			usedTokens: 64_000,
			contextLimitTokens: 128_000,
			compactionThreshold: modelThreshold(100, true),
		},
	},
};
export const Estimated: Story = {
	...Open,
	args: {
		usage: {
			usedTokens: 350,
			contextLimitTokens: 200_000,
			compactionThreshold: modelThreshold(70),
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
			compactionThreshold: modelThreshold(0),
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
