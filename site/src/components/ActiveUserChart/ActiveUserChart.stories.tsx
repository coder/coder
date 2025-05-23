import type { Meta, StoryObj } from "@storybook/react";
import { ActiveUserChart } from "./ActiveUserChart";

const meta: Meta<typeof ActiveUserChart> = {
	title: "components/ActiveUserChart",
	component: ActiveUserChart,
	args: {
		data: [
			{ date: "2024-01-01", amount: 5 },
			{ date: "2024-01-02", amount: 6 },
			{ date: "2024-01-03", amount: 7 },
			{ date: "2024-01-04", amount: 8 },
			{ date: "2024-01-05", amount: 9 },
			{ date: "2024-01-06", amount: 10 },
			{ date: "2024-01-07", amount: 11 },
		],
		interval: "day",
	},
	decorators: [
		(Story) => (
			<div style={{ height: "400px" }}>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ActiveUserChart>;

export const Example: Story = {};

export const WeeklyExample: Story = {
	args: {
		interval: "week",
		data: [
			{ date: "2024-01-01", amount: 12 },
			{ date: "2024-01-08", amount: 15 },
			{ date: "2024-01-15", amount: 18 },
			{ date: "2024-01-22", amount: 22 },
			{ date: "2024-01-29", amount: 25 },
		],
	},
};

export const ManyDataPoints: Story = {
	args: {
		data: Array.from({ length: 30 }).map((_, i) => {
			const date = new Date(2024, 0, i + 1);
			return {
				date: date.toISOString().split("T")[0],
				amount: 5 + Math.floor(Math.random() * 15),
			};
		}),
	},
};