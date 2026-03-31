import { ClockIcon, TrendingUpIcon } from "lucide-react";
import type { FC } from "react";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import type { ChatRuntimeSummary } from "#/api/typesGenerated";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "#/components/Chart/Chart";
import { Spinner } from "#/components/Spinner/Spinner";
import { formatDate } from "#/utils/time";
import { SectionHeader } from "./components/SectionHeader";

const chartConfig = {
	hours: {
		label: "Hours",
		color: "hsl(var(--highlight-purple))",
	},
} satisfies ChartConfig;

/**
 * Format milliseconds to a human-readable hours string.
 */
function formatHours(ms: number): string {
	const hours = ms / 3_600_000;
	if (hours < 0.1) {
		const minutes = ms / 60_000;
		return `${minutes.toFixed(1)}m`;
	}
	if (hours >= 1000) {
		return `${(hours / 1000).toFixed(1)}k hrs`;
	}
	return `${hours.toFixed(1)} hrs`;
}

interface AgentHoursPageViewProps {
	data: ChatRuntimeSummary | undefined;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
	rangeLabel: string;
}

export const AgentSettingsAgentHoursPageView: FC<AgentHoursPageViewProps> = ({
	data,
	isLoading,
	error,
	onRetry,
	rangeLabel,
}) => {
	const chartData =
		data?.daily.map((d) => ({
			date: d.date,
			hours: d.total_runtime_ms / 3_600_000,
		})) ?? [];

	return (
		<div className="flex flex-col gap-6">
			<SectionHeader
				label="Agent Hours"
				description="Track total agent compute time across your deployment."
				action={
					<div className="flex items-center gap-2 text-xs text-content-secondary">
						<ClockIcon className="h-4 w-4" />
						<span>{rangeLabel}</span>
					</div>
				}
			/>

			{/* Summary Cards */}
			{isLoading ? (
				<div className="flex items-center justify-center py-12">
					<Spinner loading />
				</div>
			) : error ? (
				<div className="flex flex-col items-center gap-3 py-12 text-sm text-content-secondary">
					<p className="m-0">Failed to load runtime data.</p>
					<button
						type="button"
						onClick={onRetry}
						className="cursor-pointer border-0 bg-transparent text-sm font-medium text-content-link hover:underline"
					>
						Retry
					</button>
				</div>
			) : (
				<>
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
						<div className="flex flex-col gap-1 rounded-lg border border-solid border-border p-4">
							<div className="flex items-center gap-2 text-xs font-medium text-content-secondary">
								<ClockIcon className="h-3.5 w-3.5" />
								Total in Period
							</div>
							<div className="text-2xl font-semibold text-content-primary">
								{formatHours(data?.total_runtime_ms ?? 0)}
							</div>
							<div className="text-xs text-content-secondary">
								{data?.daily.reduce((sum, d) => sum + d.message_count, 0) ?? 0}{" "}
								messages
							</div>
						</div>

						<div className="flex flex-col gap-1 rounded-lg border border-solid border-border p-4">
							<div className="flex items-center gap-2 text-xs font-medium text-content-secondary">
								<TrendingUpIcon className="h-3.5 w-3.5" />
								Projected Yearly
							</div>
							<div className="text-2xl font-semibold text-content-primary">
								{formatHours(data?.projected_yearly_runtime_ms ?? 0)}
							</div>
							<div className="text-xs text-content-secondary">
								Based on trailing{" "}
								{rangeLabel.toLowerCase().includes("day")
									? rangeLabel.toLowerCase()
									: "period"}
							</div>
						</div>
					</div>

					{/* Chart */}
					<div className="rounded-lg border border-solid border-border">
						<div className="border-0 border-b border-solid border-border p-4">
							<h3 className="m-0 text-sm font-medium text-content-primary">
								Daily Runtime
							</h3>
						</div>
						<div className="p-6">
							<div className="h-64">
								{chartData.length > 0 ? (
									<ChartContainer
										config={chartConfig}
										className="aspect-auto h-full"
									>
										<AreaChart
											accessibilityLayer
											data={chartData}
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
												dataKey="hours"
												tickLine={false}
												axisLine={false}
												tickMargin={12}
												tickFormatter={(value: number) => {
													if (value === 0) return "";
													if (value < 1) return `${(value * 60).toFixed(0)}m`;
													return `${value.toFixed(1)}h`;
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
															const hours =
																typeof item.value === "number" ? item.value : 0;
															return `${hours.toFixed(2)} hours`;
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
												<linearGradient
													id="fillHours"
													x1="0"
													y1="0"
													x2="0"
													y2="1"
												>
													<stop
														offset="5%"
														stopColor="var(--color-hours)"
														stopOpacity={0.8}
													/>
													<stop
														offset="95%"
														stopColor="var(--color-hours)"
														stopOpacity={0.1}
													/>
												</linearGradient>
											</defs>

											<Area
												isAnimationActive={false}
												dataKey="hours"
												type="linear"
												fill="url(#fillHours)"
												fillOpacity={0.4}
												stroke="var(--color-hours)"
												stackId="a"
											/>
										</AreaChart>
									</ChartContainer>
								) : (
									<div className="flex h-full w-full items-center justify-center text-sm font-medium text-content-secondary">
										No runtime data available for this period.
									</div>
								)}
							</div>
						</div>
					</div>
				</>
			)}
		</div>
	);
};
