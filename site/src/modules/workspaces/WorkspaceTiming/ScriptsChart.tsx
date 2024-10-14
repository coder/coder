import { type FC, useState } from "react";
import { Bar } from "./Chart/Bar";
import {
	Chart,
	ChartBreadcrumbs,
	ChartContent,
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
	calcDuration,
	calcOffset,
	formatTime,
	makeTicks,
	mergeTimeRanges,
	type TimeRange,
} from "./Chart/utils";
import type { StageCategory } from "./StagesChart";

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
