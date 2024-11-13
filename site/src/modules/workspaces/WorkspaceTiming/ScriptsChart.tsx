import { type Theme, useTheme } from "@emotion/react";
import { type FC, useState } from "react";
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
import { Tooltip, TooltipTitle } from "./Chart/Tooltip";
import { XAxis, XAxisRow, XAxisSection } from "./Chart/XAxis";
import {
	YAxis,
	YAxisHeader,
	YAxisLabel,
	YAxisLabels,
	YAxisSection,
} from "./Chart/YAxis";
import {
	type TimeRange,
	calcDuration,
	calcOffset,
	formatTime,
	makeTicks,
	mergeTimeRanges,
} from "./Chart/utils";
import type { Stage } from "./StagesChart";

type ScriptTiming = {
	name: string;
	status: string;
	exitCode: number;
	range: TimeRange;
};

export type ScriptsChartProps = {
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
	const visibleLegends = [...new Set(visibleTimings.map((t) => t.status))].map(
		(s) => legendsByStatus[s],
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
									<Tooltip
										title={
											<TooltipTitle>
												Script exited with <strong>code {t.exitCode}</strong>
											</TooltipTitle>
										}
									>
										<Bar
											value={duration}
											offset={calcOffset(t.range, generalTiming)}
											scale={scale}
											colors={legendsByStatus[t.status].colors}
										/>
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
		timeout: {
			label: "timed out",
			colors: {
				fill: theme.roles.warning.background,
				stroke: theme.roles.warning.outline,
			},
		},
	};
}
