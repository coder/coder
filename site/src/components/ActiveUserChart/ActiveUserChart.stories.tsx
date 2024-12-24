import type { Meta, StoryObj } from "@storybook/react";
import { ActiveUserChart } from "./ActiveUserChart";

const meta: Meta<typeof ActiveUserChart> = {
	title: "components/ActiveUserChart",
	component: ActiveUserChart,
	args: {
		series: [
			{
				label: "Daily",
				data: [
					{ date: "1/1/2024", amount: 5 },
					{ date: "1/2/2024", amount: 6 },
					{ date: "1/3/2024", amount: 7 },
					{ date: "1/4/2024", amount: 8 },
					{ date: "1/5/2024", amount: 9 },
					{ date: "1/6/2024", amount: 10 },
					{ date: "1/7/2024", amount: 11 },
				],
			},
		],
		interval: "day",
	},
};

export default meta;
type Story = StoryObj<typeof ActiveUserChart>;

export const Example: Story = {};

export const MultipleSeries: Story = {
	args: {
		series: [
			{
				label: "Active",
				data: [
					{ date: "1/1/2024", amount: 150 },
					{ date: "1/2/2024", amount: 165 },
					{ date: "1/3/2024", amount: 180 },
					{ date: "1/4/2024", amount: 155 },
					{ date: "1/5/2024", amount: 190 },
					{ date: "1/6/2024", amount: 200 },
					{ date: "1/7/2024", amount: 210 },
				],
				color: "green",
			},
			{
				label: "Dormant",
				data: [
					{ date: "1/1/2024", amount: 80 },
					{ date: "1/2/2024", amount: 82 },
					{ date: "1/3/2024", amount: 85 },
					{ date: "1/4/2024", amount: 88 },
					{ date: "1/5/2024", amount: 90 },
					{ date: "1/6/2024", amount: 92 },
					{ date: "1/7/2024", amount: 95 },
				],
				color: "grey",
			},
			{
				label: "Suspended",
				data: [
					{ date: "1/1/2024", amount: 20 },
					{ date: "1/2/2024", amount: 22 },
					{ date: "1/3/2024", amount: 25 },
					{ date: "1/4/2024", amount: 23 },
					{ date: "1/5/2024", amount: 28 },
					{ date: "1/6/2024", amount: 30 },
					{ date: "1/7/2024", amount: 32 },
				],
				color: "red",
			},
		],
		interval: "day",
		userLimit: 100,
	},
};
