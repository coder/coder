import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { AIBudgetUsage } from "./AIBudgetUsage";

// Spend and limit are in micros (1_000_000 = $1).
const meta: Meta<typeof AIBudgetUsage> = {
	title: "components/AIBudgetUsage",
	component: AIBudgetUsage,
};

export default meta;
type Story = StoryObj<typeof AIBudgetUsage>;

// No limit: spend shown against "Unlimited".
export const Unlimited: Story = {
	args: { currentSpend: 25_492_000_000, spendLimit: null },
	play: async ({ canvasElement }) => {
		await expect(canvasElement).toHaveTextContent("$25,492 / Unlimited USD");
	},
};

// Well under budget: spend rendered in the normal (secondary) color.
export const UnderBudget: Story = {
	args: { currentSpend: 10_000_000, spendLimit: 50_000_000 },
	play: async ({ canvasElement }) => {
		await expect(canvasElement).toHaveTextContent("$10 / $50 USD");
	},
};

// >=85% of budget: spend rendered in the warning color.
export const NearLimit: Story = {
	args: { currentSpend: 46_000_000, spendLimit: 50_000_000 },
	play: async ({ canvasElement }) => {
		await expect(canvasElement).toHaveTextContent("$46 / $50 USD");
	},
};

// Over budget: spend rendered in the destructive color.
export const OverBudget: Story = {
	args: { currentSpend: 75_000_000, spendLimit: 50_000_000 },
	play: async ({ canvasElement }) => {
		await expect(canvasElement).toHaveTextContent("$75 / $50 USD");
	},
};

// Zero budget with spend: treated as exceeded.
export const ZeroBudget: Story = {
	args: { currentSpend: 5_000_000, spendLimit: 0 },
	play: async ({ canvasElement }) => {
		await expect(canvasElement).toHaveTextContent("$5 / $0 USD");
	},
};
