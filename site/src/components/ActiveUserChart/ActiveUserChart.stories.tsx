import type { Meta, StoryObj } from "@storybook/react";
import { ActiveUserChart } from "./ActiveUserChart";

const meta: Meta<typeof ActiveUserChart> = {
	title: "components/ActiveUserChart",
	component: ActiveUserChart,
	args: {
		data: [
			{ date: "1/1/2024", amount: 5 },
			{ date: "1/2/2024", amount: 6 },
			{ date: "1/3/2024", amount: 7 },
			{ date: "1/4/2024", amount: 8 },
			{ date: "1/5/2024", amount: 9 },
			{ date: "1/6/2024", amount: 10 },
			{ date: "1/7/2024", amount: 11 },
		],
		interval: "day",
	},
};

export default meta;
type Story = StoryObj<typeof ActiveUserChart>;

export const Example: Story = {};
