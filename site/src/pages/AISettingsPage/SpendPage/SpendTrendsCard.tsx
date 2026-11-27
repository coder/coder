import { useId } from "react";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import type { OrganizationAISpendOverviewInterval } from "#/api/typesGenerated";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "#/components/Chart/Chart";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { formatBudgetUSD, formatCostMicros } from "#/utils/currency";
import { OverviewCard } from "./OverviewCard";
import {
	formatSpendDay,
	formatSpendDayLong,
	formatSpendDaysLabel,
	type SpendDays,
} from "./spendDays";

const chartConfig = {
	spendMicros: {
		label: "Spend",
		color: "hsl(var(--highlight-sky))",
	},
} satisfies ChartConfig;

// Labels the first, middle, and last days, so the axis stays evenly spaced
// for any number of days.
const edgeAndMiddleTicks = (data: readonly { day: string }[]): string[] => {
	const middle = Math.floor((data.length - 1) / 2);
	return data
		.filter((_, i) => i === 0 || i === middle || i === data.length - 1)
		.map((d) => d.day);
};

type SpendTrendsCardProps = {
	days: SpendDays;
	/** Undefined while the spend loads. */
	intervals: readonly OrganizationAISpendOverviewInterval[] | undefined;
};

export const SpendTrendsCard: React.FC<SpendTrendsCardProps> = ({
	days,
	intervals,
}) => {
	const titleId = useId();
	const label = formatSpendDaysLabel(days);
	const data = intervals?.map((interval) => ({
		day: interval.start_time,
		spendMicros: interval.spend_micros,
	}));

	return (
		<OverviewCard aria-labelledby={titleId} className="gap-6">
			<div className="flex flex-col gap-2">
				<h2 id={titleId} className="m-0 text-xl font-semibold">
					AI spend trends
				</h2>
				<p className="m-0 text-sm text-content-secondary">
					Based on daily spend from {label} (UTC).
				</p>
			</div>
			{data === undefined ? (
				<Skeleton className="h-80" />
			) : data.every((day) => day.spendMicros === 0) ? (
				<div className="flex h-80 items-center justify-center text-sm font-medium text-content-secondary">
					No AI spend in this period.
				</div>
			) : (
				<ChartContainer config={chartConfig} className="aspect-auto h-80">
					<BarChart
						accessibilityLayer
						role="group"
						aria-roledescription="chart"
						aria-labelledby={titleId}
						desc={`Daily AI spend from ${label} (UTC). Use the left and right arrow keys to read each day.`}
						data={data}
						margin={{ top: 10, left: 0, right: 24 }}
					>
						<CartesianGrid vertical={false} />
						<XAxis
							dataKey="day"
							tickLine={false}
							tickMargin={12}
							ticks={edgeAndMiddleTicks(data)}
							interval={0}
							// Each interval is a whole UTC day, so its label must not
							// shift to the viewer's timezone.
							tickFormatter={formatSpendDay}
						/>
						<YAxis
							tickLine={false}
							axisLine={false}
							tickMargin={12}
							tickFormatter={(micros: number) =>
								micros === 0 ? "" : formatBudgetUSD(micros)
							}
						/>
						<ChartTooltip
							cursor={false}
							content={(props) => (
								<ChartTooltipContent
									{...props}
									hideIndicator
									className="font-medium text-content-secondary"
									labelClassName="text-content-primary"
									labelFormatter={(_, payload) =>
										formatSpendDayLong(payload[0].payload.day)
									}
									formatter={(micros) => formatCostMicros(Number(micros))}
								/>
							)}
						/>
						<Bar
							isAnimationActive={false}
							dataKey="spendMicros"
							fill="var(--color-spendMicros)"
							radius={2}
							maxBarSize={8}
						/>
					</BarChart>
				</ChartContainer>
			)}
		</OverviewCard>
	);
};
