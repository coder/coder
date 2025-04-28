import type { Meta, StoryObj } from "@storybook/react";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "./Chart";

const chartData = [
	{ month: "January", users: 186 },
	{ month: "February", users: 305 },
	{ month: "March", users: 237 },
	{ month: "April", users: 73 },
	{ month: "May", users: 209 },
	{ month: "June", users: 214 },
];

const chartConfig = {
	users: {
		label: "Users",
		color: "hsl(var(--highlight-purple))",
	},
} satisfies ChartConfig;

const meta: Meta<typeof ChartContainer> = {
	title: "components/Chart",
	render: () => {
		return (
			<ChartContainer config={chartConfig}>
				<AreaChart
					accessibilityLayer
					data={chartData}
					margin={{
						left: 12,
						right: 12,
					}}
				>
					<CartesianGrid vertical={false} />
					<XAxis
						dataKey="month"
						tickLine={false}
						axisLine={false}
						tickMargin={8}
						tickFormatter={(value) => value.slice(0, 3)}
					/>
					<YAxis
						dataKey="users"
						tickLine={false}
						axisLine={false}
						tickMargin={8}
						tickFormatter={(value: number) => value.toLocaleString()}
					/>
					<ChartTooltip
						cursor={false}
						content={<ChartTooltipContent indicator="dot" />}
					/>
					<Area
						dataKey="users"
						type="natural"
						fill="var(--color-users)"
						fillOpacity={0.4}
						stroke="var(--color-users)"
						stackId="a"
					/>
				</AreaChart>
			</ChartContainer>
		);
	},
};

export default meta;
type Story = StoryObj<typeof ChartContainer>;

export const Default: Story = {};
