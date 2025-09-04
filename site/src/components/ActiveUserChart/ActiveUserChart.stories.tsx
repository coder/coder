import type { Meta, StoryObj } from "@storybook/react-vite";
import { ActiveUserChart } from "./ActiveUserChart";

const meta: Meta<typeof ActiveUserChart> = {
	title: "components/ActiveUserChart",
	component: ActiveUserChart,
	args: {
		data: [
			{ date: "2024-01-01", amount: 12 },
			{ date: "2024-01-02", amount: 8 },
			{ date: "2024-01-03", amount: 15 },
			{ date: "2024-01-04", amount: 3 },
			{ date: "2024-01-05", amount: 22 },
			{ date: "2024-01-06", amount: 7 },
			{ date: "2024-01-07", amount: 18 },
		],
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

export const ManyDataPoints: Story = {
	args: {
		data: [
			{ date: "2024-01-01", amount: 12 },
			{ date: "2024-01-02", amount: 8 },
			{ date: "2024-01-03", amount: 15 },
			{ date: "2024-01-04", amount: 3 },
			{ date: "2024-01-05", amount: 22 },
			{ date: "2024-01-06", amount: 7 },
			{ date: "2024-01-07", amount: 18 },
			{ date: "2024-01-08", amount: 31 },
			{ date: "2024-01-09", amount: 5 },
			{ date: "2024-01-10", amount: 27 },
			{ date: "2024-01-11", amount: 14 },
			{ date: "2024-01-12", amount: 9 },
			{ date: "2024-01-13", amount: 35 },
			{ date: "2024-01-14", amount: 21 },
			{ date: "2024-01-15", amount: 6 },
			{ date: "2024-01-16", amount: 29 },
			{ date: "2024-01-17", amount: 11 },
			{ date: "2024-01-18", amount: 17 },
			{ date: "2024-01-19", amount: 4 },
			{ date: "2024-01-20", amount: 25 },
			{ date: "2024-01-21", amount: 13 },
			{ date: "2024-01-22", amount: 33 },
			{ date: "2024-01-23", amount: 19 },
			{ date: "2024-01-24", amount: 26 },
		],
	},
};
