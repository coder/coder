import type { Meta, StoryObj } from "@storybook/react-vite";
import { NetworkCallBadges } from "./NetworkCallBadges";

const meta: Meta<typeof NetworkCallBadges> = {
	title: "pages/AIBridgePage/NetworkCallBadges",
	component: NetworkCallBadges,
};

export default meta;
type Story = StoryObj<typeof NetworkCallBadges>;

export const TotalAndBlocked: Story = {
	args: {
		summary: { total: 23, blocked: 2, errored: 0 },
	},
};

export const WithErrors: Story = {
	args: {
		summary: { total: 23, blocked: 2, errored: 1 },
	},
};

export const NoBlocked: Story = {
	args: {
		summary: { total: 23, blocked: 0, errored: 0 },
	},
};

export const NoActivity: Story = {
	args: {
		summary: { total: 0, blocked: 0, errored: 0 },
	},
};

export const Disabled: Story = {
	args: {
		summary: undefined,
	},
};

export const LargeCounts: Story = {
	args: {
		summary: { total: 12_480, blocked: 320, errored: 47 },
	},
};

export const SizeXs: Story = {
	args: {
		size: "xs",
		summary: { total: 23, blocked: 2, errored: 1 },
	},
};

export const SizeMd: Story = {
	args: {
		size: "md",
		summary: { total: 23, blocked: 2, errored: 1 },
	},
};
