import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { UsageBar } from "./UsageBar";

const meta: Meta<typeof UsageBar> = {
	title: "components/UsageBar",
	component: UsageBar,
	args: {
		ariaLabel: "Usage",
		percent: 50,
		className: "w-60",
	},
};

export default meta;
type Story = StoryObj<typeof UsageBar>;

export const Empty: Story = {
	args: { percent: 0, severity: "normal" },
};

export const Normal: Story = {
	args: { percent: 50, severity: "normal" },
};

export const Warning: Story = {
	args: { percent: 90, severity: "warning" },
};

export const Exceeded: Story = {
	args: { percent: 100, severity: "exceeded" },
};

export const ClampsOutOfRange: Story = {
	args: { percent: 150, severity: "exceeded" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("progressbar")).toHaveAttribute(
			"aria-valuenow",
			"100",
		);
	},
};
