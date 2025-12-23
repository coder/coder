import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "components/Chart/Chart";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipIconTrigger,
	HelpTooltipText,
	HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import type { FC } from "react";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";

const chartConfig = {
	amount: {
		label: "Active Users",
		color: "hsl(var(--highlight-purple))",
	},
} satisfies ChartConfig;
interface ActiveUserChartProps {
	data: { date: string; amount: number }[];
}

export const ActiveUserChart: FC<ActiveUserChartProps> = ({ data }) => {
	return (
		<ChartContainer config={chartConfig} className="aspect-auto h-full">
			<AreaChart
				accessibilityLayer
				data={data}
				margin={{
					top: 10,
					left: 0,
					right: 0,
				}}
			>
				<CartesianGrid vertical={false} />
				<XAxis
					dataKey="date"
					tickLine={false}
					tickMargin={12}
					minTickGap={24}
					tickFormatter={(value: string) =>
						new Date(value).toLocaleDateString(undefined, {
							month: "short",
							day: "numeric",
						})
					}
				/>
				<YAxis
					dataKey="amount"
					tickLine={false}
					axisLine={false}
					tickMargin={12}
					tickFormatter={(value: number) => {
						return value === 0 ? "" : value.toLocaleString();
					}}
				/>
				<ChartTooltip
					cursor={false}
					content={
						<ChartTooltipContent
							className="font-medium text-content-secondary"
							labelClassName="text-content-primary"
							labelFormatter={(_, p) => {
								const item = p[0];
								return `${item?.value} active users`;
							}}
							formatter={(_v, _n, item) => {
								const date = new Date(item.payload.date);
								return date.toLocaleString(undefined, {
									month: "long",
									day: "2-digit",
								});
							}}
						/>
					}
				/>
				<defs>
					<linearGradient id="fillAmount" x1="0" y1="0" x2="0" y2="1">
						<stop
							offset="5%"
							stopColor="var(--color-amount)"
							stopOpacity={0.8}
						/>
						<stop
							offset="95%"
							stopColor="var(--color-amount)"
							stopOpacity={0.1}
						/>
					</linearGradient>
				</defs>

				<Area
					isAnimationActive={false}
					dataKey="amount"
					type="linear"
					fill="url(#fillAmount)"
					fillOpacity={0.4}
					stroke="var(--color-amount)"
					stackId="a"
				/>
			</AreaChart>
		</ChartContainer>
	);
};

type ActiveUsersTitleProps = {
	interval: "day" | "week";
};

export const ActiveUsersTitle: FC<ActiveUsersTitleProps> = ({ interval }) => {
	return (
		<div className="flex items-center gap-2">
			{interval === "day" ? "Daily" : "Weekly"} Active Users
			<HelpTooltip>
				<HelpTooltipIconTrigger size="small" />
				<HelpTooltipContent>
					<HelpTooltipTitle>How do we calculate active users?</HelpTooltipTitle>
					<HelpTooltipText>
						When a connection is initiated to a user&apos;s workspace they are
						considered an active user. e.g. apps, web terminal, SSH. This is for
						measuring user activity and has no connection to license
						consumption.
					</HelpTooltipText>
				</HelpTooltipContent>
			</HelpTooltip>
		</div>
	);
};
