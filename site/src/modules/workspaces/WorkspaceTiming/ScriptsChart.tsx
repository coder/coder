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
import type { StageCategory } from "./StagesChart";

const legendsByStatus: Record<string, ChartLegend> = {
	ok: {
		label: "success",
		colors: {
			fill: "#022C22",
			stroke: "#BBF7D0",
		},
	},
	exit_failure: {
		label: "failure",
		colors: {
			fill: "#450A0A",
			stroke: "#F87171",
		},
	},
	timeout: {
		label: "timed out",
		colors: {
			fill: "#422006",
			stroke: "#FDBA74",
		},
	},
};

type ScriptTiming = {
	name: string;
	status: string;
	exitCode: number;
	range: TimeRange;
};

export type ScriptsChartProps = {
	category: StageCategory;
	stage: string;
	timings: ScriptTiming[];
	onBack: () => void;
};

export const ScriptsChart: FC<ScriptsChartProps> = ({
	category,
	stage,
	timings,
	onBack,
}) => {
	const generalTiming = mergeTimeRanges(timings.map((t) => t.range));
	const totalTime = calcDuration(generalTiming);
	const [ticks, scale] = makeTicks(totalTime);
	const [filter, setFilter] = useState("");
	const visibleTimings = timings.filter((t) => t.name.includes(filter));
	const visibleLegends = [...new Set(visibleTimings.map((t) => t.status))].map(
		(s) => legendsByStatus[s],
	);

	return (
		<Chart>
			<ChartToolbar>
				<ChartBreadcrumbs
					breadcrumbs={[
						{
							label: category.name,
							onClick: onBack,
						},
						{
							label: stage,
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
						<YAxisHeader>{stage} stage</YAxisHeader>
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
