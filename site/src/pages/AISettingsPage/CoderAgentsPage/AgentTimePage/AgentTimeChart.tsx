import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import type { AgentTimeBucket, AgentTimeReport } from "#/api/typesGenerated";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "#/components/Chart/Chart";
import {
	formatAgentTimeHours,
	formatBucketRange,
	msToHours,
} from "./agentTimeUtils";

const chartConfig = {
	hours: {
		label: "Agent time",
		color: "hsl(var(--highlight-purple))",
	},
} satisfies ChartConfig;

type ChartDatum = {
	bucket: AgentTimeBucket;
	label: string;
	hours: number | null;
};

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null;
}

function isAgentTimeBucket(value: unknown): value is AgentTimeBucket {
	return (
		isRecord(value) &&
		typeof value.start_date === "string" &&
		typeof value.end_date === "string" &&
		(typeof value.agent_time_ms === "string" || value.agent_time_ms === null) &&
		typeof value.partial === "boolean" &&
		typeof value.complete === "boolean"
	);
}

function chartDatumFromPayload(value: unknown): ChartDatum | undefined {
	if (
		isRecord(value) &&
		isAgentTimeBucket(value.bucket) &&
		typeof value.label === "string" &&
		(typeof value.hours === "number" || value.hours === null)
	) {
		return {
			bucket: value.bucket,
			label: value.label,
			hours: value.hours,
		};
	}
	return undefined;
}

export function AgentTimeChart({ report }: { report: AgentTimeReport }) {
	const data: ChartDatum[] = report.buckets.map((bucket) => ({
		bucket,
		label: `${formatBucketRange(bucket)}${bucket.partial ? " (partial)" : ""}`,
		hours: msToHours(bucket.agent_time_ms),
	}));

	return (
		<ChartContainer config={chartConfig} className="aspect-auto h-[260px]">
			<BarChart
				accessibilityLayer
				data={data}
				margin={{ top: 8, right: 8, bottom: 0, left: -12 }}
			>
				<CartesianGrid vertical={false} />
				<XAxis
					dataKey="label"
					tickLine={false}
					tickMargin={12}
					minTickGap={28}
				/>
				<YAxis
					tickLine={false}
					axisLine={false}
					tickMargin={4}
					width={44}
					tickFormatter={(value: number) =>
						value === 0 ? "" : `${String(value)}h`
					}
				/>
				<ChartTooltip
					cursor={false}
					content={
						<ChartTooltipContent
							labelFormatter={(_value, payload) => {
								const item = payload?.find(
									(entry) => chartDatumFromPayload(entry.payload) !== undefined,
								);
								const datum = chartDatumFromPayload(item?.payload);
								return datum?.label ?? "";
							}}
							formatter={(_value, _name, item) => {
								const datum = chartDatumFromPayload(item.payload);
								return (
									<div className="flex w-full items-center justify-between gap-4">
										<span className="text-content-secondary">Agent time</span>
										<span className="font-mono font-medium tabular-nums text-content-primary">
											{formatAgentTimeHours(datum?.bucket.agent_time_ms)}
										</span>
									</div>
								);
							}}
						/>
					}
				/>
				<Bar
					dataKey="hours"
					fill="var(--color-hours)"
					radius={[4, 4, 0, 0]}
					isAnimationActive={false}
				/>
			</BarChart>
		</ChartContainer>
	);
}
