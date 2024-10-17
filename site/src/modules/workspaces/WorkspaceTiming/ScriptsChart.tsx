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
import { XAxis, XAxisRow, XAxisSection } from "./Chart/XAxis";
import {
	YAxis,
	YAxisCaption,
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
			fill: "#341B1D",
			stroke: "#EF4547",
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
						<YAxisCaption>{stage} stage</YAxisCaption>
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
									<Bar
										value={duration}
										offset={calcOffset(t.range, generalTiming)}
										scale={scale}
										colors={legendsByStatus[t.status].colors}
									/>

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
