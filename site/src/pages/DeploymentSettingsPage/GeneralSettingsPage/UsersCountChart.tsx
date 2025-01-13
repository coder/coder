import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import type { FC } from "react";

const chartConfig = {
	desktop: {
		label: "Desktop",
		color: "hsl(var(--chart-1))",
	},
	mobile: {
		label: "Mobile",
		color: "hsl(var(--chart-2))",
	},
} satisfies ChartConfig;

type Data = {
	date: string;
	amount: number;
};

export type UsersCountChartProps = {
	active: Data[];
	dormant: Data[];
	suspended: Data[];
};

export const UsersCountChart: FC<UsersCountChartProps> = () => {
	return (
		<div className="border border-solid rounded-sm">
			<div className="p-4">
				<Collapsible>
					<h3>User Engagement</h3>
					<CollapsibleTrigger asChild>
						<button type="button">How we calculate engaged users</button>
					</CollapsibleTrigger>
					<CollapsibleContent className="px-5">
						<p>
							We consider a user “engaged” if they initiate a connection to
							their workspace. The connection can be made through apps, web
							terminal or SSH.
						</p>
						<p>
							The graph shows the number of unique users who were engaged at
							least once during the day.
						</p>
						<p>You might also check:</p>
						<ul>
							<li>Activity Audit</li>
							<li>License Consumption</li>
						</ul>
					</CollapsibleContent>
				</Collapsible>
			</div>

			<div className="p-6">
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
						<ChartTooltip
							cursor={false}
							content={<ChartTooltipContent indicator="dot" />}
						/>
						<Area
							dataKey="mobile"
							type="natural"
							fill="var(--color-mobile)"
							fillOpacity={0.4}
							stroke="var(--color-mobile)"
							stackId="a"
						/>
						<Area
							dataKey="desktop"
							type="natural"
							fill="var(--color-desktop)"
							fillOpacity={0.4}
							stroke="var(--color-desktop)"
							stackId="a"
						/>
					</AreaChart>
				</ChartContainer>
			</div>
		</div>
	);
};
