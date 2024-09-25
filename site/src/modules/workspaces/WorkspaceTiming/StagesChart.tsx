import {
	XAxis,
	XAxisRow,
	XAxisRows,
	XAxisSections,
	XAxisMinWidth,
} from "./Chart/XAxis";
import type { FC } from "react";
import {
	YAxis,
	YAxisCaption,
	YAxisLabel,
	YAxisLabels,
	YAxisSection,
} from "./Chart/YAxis";
import { Bar, ClickableBar } from "./Chart/Bar";
import {
	calcDuration,
	calcOffset,
	combineTimings,
	formatTime,
	makeTicks,
	type BaseTiming,
} from "./Chart/utils";
import { Chart, ChartContent } from "./Chart/Chart";
import { BarBlocks } from "./Chart/BarBlocks";

// TODO: Add "workspace boot" when scripting timings are done.
const stageCategories = ["provisioning"] as const;

type StageCategory = (typeof stageCategories)[number];

type Stage = { name: string; category: StageCategory };

// TODO: Export provisioning stages from the BE to the generated types.
export const stages: Stage[] = [
	{
		name: "init",
		category: "provisioning",
	},
	{
		name: "plan",
		category: "provisioning",
	},
	{
		name: "graph",
		category: "provisioning",
	},
	{
		name: "apply",
		category: "provisioning",
	},
];

type StageTiming = BaseTiming & {
	name: string;
	/**
	 * Represents the number of resources included in this stage. This value is
	 * used to display individual blocks within the bar, indicating that the stage
	 * consists of multiple resource time blocks.
	 */
	resources: number;
	/**
	 * Represents the category of the stage. This value is used to group stages
	 * together in the chart. For example, all provisioning stages are grouped
	 * together.
	 */
	category: StageCategory;
};

export type StagesChartProps = {
	timings: StageTiming[];
	onSelectStage: (timing: StageTiming, category: StageCategory) => void;
};

export const StagesChart: FC<StagesChartProps> = ({
	timings,
	onSelectStage,
}) => {
	const generalTiming = combineTimings(timings);
	const totalTime = calcDuration(generalTiming);
	const [ticks, scale] = makeTicks(totalTime);

	return (
		<Chart>
			<ChartContent>
				<YAxis>
					{stageCategories.map((c) => {
						const stagesInCategory = stages.filter((s) => s.category === c);

						return (
							<YAxisSection key={c}>
								<YAxisCaption>{c}</YAxisCaption>
								<YAxisLabels>
									{stagesInCategory.map((stage) => (
										<YAxisLabel key={stage.name} id={stage.name}>
											{stage.name}
										</YAxisLabel>
									))}
								</YAxisLabels>
							</YAxisSection>
						);
					})}
				</YAxis>

				<XAxis ticks={ticks} scale={scale}>
					<XAxisSections>
						{stageCategories.map((category) => {
							const timingsInCategory = timings.filter(
								(t) => t.category === category,
							);
							return (
								<XAxisRows key={category}>
									{timingsInCategory.map((t) => {
										const value = calcDuration(t);
										const offset = calcOffset(t, generalTiming);

										return (
											<XAxisRow key={t.name} yAxisLabelId={t.name}>
												{/** We only want to expand stages with more than one resource */}
												{t.resources > 1 ? (
													<ClickableBar
														scale={scale}
														value={value}
														offset={offset}
														onClick={() => {
															onSelectStage(t, category);
														}}
													>
														<BarBlocks count={t.resources} />
													</ClickableBar>
												) : (
													<Bar scale={scale} value={value} offset={offset} />
												)}
												{formatTime(calcDuration(t), scale)}
											</XAxisRow>
										);
									})}
								</XAxisRows>
							);
						})}
					</XAxisSections>
				</XAxis>
			</ChartContent>
		</Chart>
	);
};
