import {
	Area,
	AreaChart,
	CartesianGrid,
	Dot,
	type DotProps,
	XAxis,
	YAxis,
} from "recharts";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
	getDailyChartDescription,
} from "#/components/Chart/Chart";
import { formatDate } from "#/utils/time";

const chartConfig = {
	amount: {
		label: "Active users",
		color: "hsl(var(--highlight-purple))",
	},
} satisfies ChartConfig;

type ActiveUserDay = { date: string; amount: number };

const formatDay = (date: string) =>
	new Date(date).toLocaleString(undefined, {
		month: "long",
		day: "2-digit",
	});

type ActiveUserChartProps = {
	data: ActiveUserDay[];
};

export const ActiveUserChart: React.FC<ActiveUserChartProps> = ({ data }) => {
	return (
		<ChartContainer config={chartConfig} className="aspect-auto h-full">
			<AreaChart
				accessibilityLayer
				role="group"
				aria-roledescription="chart"
				title="Active users"
				desc={getDailyChartDescription(
					"Daily active users",
					data.map((d) => d.date),
				)}
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
						formatDate(new Date(value), {
							month: "short",
							day: "numeric",
							year: undefined,
							hour: undefined,
							minute: undefined,
							second: undefined,
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
					content={(props) => (
						<ChartTooltipContent
							{...props}
							className="font-medium text-content-secondary"
							labelClassName="text-content-primary"
							labelFormatter={(_, p) => {
								const item = p[0];
								return `${item.value} active users`;
							}}
							formatter={(_v, _n, item) => formatDay(item.payload.date)}
						/>
					)}
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
					// Screen readers in browse mode reach the highlighted point
					// directly, so it carries the same text as the tooltip.
					activeDot={({
						payload,
						...props
					}: DotProps & { payload: ActiveUserDay }) => (
						<Dot
							{...props}
							role="img"
							aria-label={`${payload.amount} active users, ${formatDay(payload.date)}`}
						/>
					)}
				/>
			</AreaChart>
		</ChartContainer>
	);
};
