import { type Theme, useTheme } from "@emotion/react";
import { type FC, useState } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { Bar } from "./Chart/Bar";
import {
	Chart,
	ChartBreadcrumbs,
	ChartContent,
	type ChartLegend,
	ChartLegends,
	ChartSearch,
	ChartToolbar,
} from "./Chart/Chart";
import {
	calcDuration,
	calcOffset,
	formatTime,
	makeTicks,
	mergeTimeRanges,
	type TimeRange,
} from "./Chart/utils";
import { XAxis, XAxisRow, XAxisSection } from "./Chart/XAxis";
import {
	YAxis,
	YAxisHeader,
	YAxisLabel,
	YAxisLabels,
	YAxisSection,
} from "./Chart/YAxis";
import type { Stage } from "./StagesChart";

type ScriptTiming = {
	name: string;
	status: string;
	exitCode: number;
	range: TimeRange;
};

type ScriptsChartProps = {
	stage: Stage;
	timings: ScriptTiming[];
	onBack: () => void;
};

export const ScriptsChart: FC<ScriptsChartProps> = ({
	stage,
	timings,
	onBack,
}) => {
	const generalTiming = mergeTimeRanges(timings.map((t) => t.range));
	const totalTime = calcDuration(generalTiming);
	const [ticks, scale] = makeTicks(totalTime);
	const [filter, setFilter] = useState("");
	const visibleTimings = timings.filter((t) => t.name.includes(filter));
	const theme = useTheme();
	const legendsByStatus = getLegendsByStatus(theme);
	// Unknown statuses fall back to a neutral legend instead of crashing
	// the chart when the backend adds a status before the UI learns it.
	const getLegend = (status: string): ChartLegend =>
		legendsByStatus[status] ?? {
			label: status.replaceAll("_", " "),
			colors: {
				fill: theme.roles.inactive.background,
				stroke: theme.roles.inactive.outline,
			},
		};
	const visibleLegends = [...new Set(visibleTimings.map((t) => t.status))].map(
		getLegend,
	);

	return (
		<Chart>
			<ChartToolbar>
				<ChartBreadcrumbs
					breadcrumbs={[
						{
							label: stage.section,
							onClick: onBack,
						},
						{
							label: stage.name,
						},
					]}
				/>
				<ChartSearch
					placeholder="Filter results..."
					value={filter}
					onChange={setFilter}
				/>
				<ChartLegends legends={visibleLegends} />
			</ChartToolbar>
			<ChartContent>
				<YAxis>
					<YAxisSection>
						<YAxisHeader>{stage.name} stage</YAxisHeader>
						<YAxisLabels>
							{visibleTimings.map((t) => (
								<YAxisLabel key={t.name} id={encodeURIComponent(t.name)}>
									{t.name}
								</YAxisLabel>
							))}
						</YAxisLabels>
					</YAxisSection>
				</YAxis>

				<XAxis ticks={ticks} scale={scale}>
					<XAxisSection>
						{visibleTimings.map((t) => {
							const duration = calcDuration(t.range);

							return (
								<XAxisRow
									key={t.name}
									yAxisLabelId={encodeURIComponent(t.name)}
								>
									<Tooltip>
										<TooltipTrigger asChild>
											<Bar
												value={duration}
												offset={calcOffset(t.range, generalTiming)}
												scale={scale}
												colors={getLegend(t.status).colors}
											/>
										</TooltipTrigger>
										<TooltipContent
											side="bottom"
											className="border-surface-quaternary text-content-primary"
										>
											{t.status === "skipped" ? (
												<>
													Script was <strong>skipped</strong> because a
													dependency did not succeed
												</>
											) : (
												<>
													Script exited with <strong>code {t.exitCode}</strong>
												</>
											)}
										</TooltipContent>
									</Tooltip>

									{formatTime(duration)}
								</XAxisRow>
							);
						})}
					</XAxisSection>
				</XAxis>
			</ChartContent>
		</Chart>
	);
};

function getLegendsByStatus(theme: Theme): Record<string, ChartLegend> {
	return {
		ok: {
			label: "success",
			colors: {
				fill: theme.roles.success.background,
				stroke: theme.roles.success.outline,
			},
		},
		exit_failure: {
			label: "failure",
			colors: {
				fill: theme.roles.error.background,
				stroke: theme.roles.error.outline,
			},
		},
		timed_out: {
			label: "timed out",
			colors: {
				fill: theme.roles.warning.background,
				stroke: theme.roles.warning.outline,
			},
		},
		pipes_left_open: {
			label: "pipes left open",
			colors: {
				fill: theme.roles.notice.background,
				stroke: theme.roles.notice.outline,
			},
		},
		skipped: {
			label: "skipped",
			colors: {
				fill: theme.roles.inactive.background,
				stroke: theme.roles.inactive.outline,
			},
		},
	};
}
