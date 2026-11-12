import { useId } from "react";
import {
	Area,
	CartesianGrid,
	ComposedChart,
	ReferenceLine,
	XAxis,
	YAxis,
} from "recharts";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "#/components/Chart/Chart";
import { type DemoRunner, getRunnerHistory } from "./demoData";

const chartConfig = {
	busy: { label: "In use", color: "hsl(var(--highlight-purple))" },
	idle: { label: "Ready", color: "hsl(var(--highlight-green))" },
	starting: { label: "Starting", color: "hsl(var(--highlight-sky))" },
} satisfies ChartConfig;

type RunnerUsageChartProps = {
	runner: DemoRunner;
	now: number;
	days: number;
};

/** Plots capacity over the selected days ending at the reference timestamp. */
export const RunnerUsageChart: React.FC<RunnerUsageChartProps> = ({
	runner,
	now,
	days,
}) => {
	const gradientId = useId().replace(/:/g, "");
	return (
		<>
			<ChartContainer config={chartConfig} className="h-72 w-full aspect-auto">
				<ComposedChart
					data={getRunnerHistory(runner, now, days)}
					accessibilityLayer
					role="group"
					aria-roledescription="chart"
					title="Runner usage"
					desc={`Concurrent runners in use, ready, and starting over the last ${days} ${days === 1 ? "day" : "days"}. Maximum capacity: ${runner.maxRunners}.`}
					margin={{ top: 24, right: 12, bottom: 4, left: -20 }}
				>
					<defs>
						{Object.entries(chartConfig).map(([key, config]) => (
							<linearGradient
								key={key}
								id={`${gradientId}-${key}`}
								x1="0"
								y1="0"
								x2="0"
								y2="1"
							>
								<stop offset="0%" stopColor={config.color} stopOpacity={0.3} />
								<stop
									offset="100%"
									stopColor={config.color}
									stopOpacity={0.04}
								/>
							</linearGradient>
						))}
					</defs>
					<CartesianGrid vertical={false} strokeDasharray="3 3" />
					<XAxis
						dataKey="date"
						tickLine={false}
						axisLine={false}
						tickMargin={12}
						minTickGap={48}
						tickFormatter={(value: number) =>
							new Date(value).toLocaleString(
								"en-US",
								days === 1
									? { hour: "numeric", hour12: true }
									: { month: "short", day: "numeric" },
							)
						}
					/>
					<YAxis
						tickLine={false}
						axisLine={false}
						tickMargin={8}
						allowDecimals={false}
						domain={[0, runner.maxRunners]}
					/>
					<ChartTooltip
						content={(props) => (
							<ChartTooltipContent
								{...props}
								labelFormatter={(_label, payload) => {
									const timestamp: unknown = payload[0]?.payload?.date;
									if (
										typeof timestamp !== "number" ||
										!Number.isFinite(timestamp)
									) {
										return "Unknown time";
									}
									return new Date(timestamp).toLocaleString("en-US", {
										month: "short",
										day: "numeric",
										hour: "numeric",
										minute: "2-digit",
									});
								}}
							/>
						)}
					/>
					<ReferenceLine
						y={runner.maxRunners}
						stroke="hsl(var(--content-secondary))"
						strokeDasharray="4 4"
						label={{
							value: `Maximum: ${runner.maxRunners}`,
							position: "insideTopRight",
							fill: "hsl(var(--content-secondary))",
							fontSize: 11,
						}}
					/>
					{Object.keys(chartConfig).map((key) => (
						<Area
							key={key}
							dataKey={key}
							type="stepAfter"
							stackId="runners"
							stroke={key === "busy" ? "var(--color-busy)" : "none"}
							strokeWidth={2}
							fill={`url(#${gradientId}-${key})`}
							isAnimationActive={false}
						/>
					))}
				</ComposedChart>
			</ChartContainer>
			<div className="mt-6 flex flex-wrap items-center justify-center gap-6 text-xs text-content-secondary">
				{Object.entries(chartConfig).map(([key, config]) => (
					<span key={key} className="flex items-center gap-2">
						<span
							className="size-2 rounded-sm"
							style={{ backgroundColor: config.color }}
						/>
						{config.label}
					</span>
				))}
				<span className="flex items-center gap-2">
					<span className="w-4 border-t border-dashed border-content-secondary" />
					Maximum capacity
				</span>
			</div>
		</>
	);
};
