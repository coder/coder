import type { Interpolation, Theme } from "@emotion/react";
import ErrorSharp from "@mui/icons-material/ErrorSharp";
import InfoOutlined from "@mui/icons-material/InfoOutlined";
import { Tooltip, type TooltipProps, TooltipTitle } from "./Chart/Tooltip";
import type { FC } from "react";
import { Bar, ClickableBar } from "./Chart/Bar";
import { Blocks } from "./Chart/Blocks";
import { Chart, ChartContent } from "./Chart/Chart";
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

export type StageCategory = {
	name: string;
	id: "provisioning" | "workspaceBoot";
};

const stageCategories: StageCategory[] = [
	{
		name: "provisioning",
		id: "provisioning",
	},
	{
		name: "workspace boot",
		id: "workspaceBoot",
	},
] as const;

export type Stage = {
	name: string;
	categoryID: StageCategory["id"];
	tooltip: Omit<TooltipProps, "children">;
};

export const stages: Stage[] = [
	{
		name: "init",
		categoryID: "provisioning",
		tooltip: {
			title: (
				<>
					<TooltipTitle>Terraform initialization</TooltipTitle>
					<span>Download providers & modules.</span>
				</>
			),
		},
	},
	{
		name: "plan",
		categoryID: "provisioning",
		tooltip: {
			title: (
				<>
					<TooltipTitle>Terraform plan</TooltipTitle>
					<span>
						Compare state of desired vs actual resources and compute changes to
						be made.
					</span>
				</>
			),
		},
	},
	{
		name: "graph",
		categoryID: "provisioning",
		tooltip: {
			title: (
				<>
					<TooltipTitle>Terraform graph</TooltipTitle>
					<span>
						List all resources in plan, used to update coderd database.
					</span>
				</>
			),
		},
	},
	{
		name: "apply",
		categoryID: "provisioning",
		tooltip: {
			title: (
				<>
					<TooltipTitle>Terraform apply</TooltipTitle>
					<span>
						Execute terraform plan to create/modify/delete resources into
						desired states.
					</span>
				</>
			),
		},
	},
	{
		name: "start",
		categoryID: "workspaceBoot",
		tooltip: {
			title: (
				<>
					<TooltipTitle>Start</TooltipTitle>
					<span>Scripts executed when the agent is starting.</span>
				</>
			),
		},
	},
];

type StageTiming = {
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
	categoryID: StageCategory["id"];
	/**
	 * Represents the time range of the stage. This value is used to calculate the
	 * duration of the stage and to position the stage within the chart. This can
	 * be undefined if a stage has no timing data.
	 */
	range: TimeRange | undefined;
	/**
	 * Display an error icon within the bar to indicate when a stage has failed.
	 * This is used in the agent scripts stage.
	 */
	error?: boolean;
};

export type StagesChartProps = {
	timings: StageTiming[];
	onSelectStage: (timing: StageTiming, category: StageCategory) => void;
};

export const StagesChart: FC<StagesChartProps> = ({
	timings,
	onSelectStage,
}) => {
	const totalRange = mergeTimeRanges(
		timings.map((t) => t.range).filter((t) => t !== undefined),
	);
	const totalTime = calcDuration(totalRange);
	const [ticks, scale] = makeTicks(totalTime);

	return (
		<Chart>
			<ChartContent>
				<YAxis>
					{stageCategories.map((c) => {
						const stagesInCategory = stages.filter(
							(s) => s.categoryID === c.id,
						);

						return (
							<YAxisSection key={c.id}>
								<YAxisHeader>{c.name}</YAxisHeader>
								<YAxisLabels>
									{stagesInCategory.map((stage) => (
										<YAxisLabel
											key={stage.name}
											id={encodeURIComponent(stage.name)}
										>
											<span css={styles.stageLabel}>
												{stage.name}
												<Tooltip {...stage.tooltip}>
													<InfoOutlined css={styles.info} />
												</Tooltip>
											</span>
										</YAxisLabel>
									))}
								</YAxisLabels>
							</YAxisSection>
						);
					})}
				</YAxis>

				<XAxis ticks={ticks} scale={scale}>
					{stageCategories.map((category) => {
						const stageTimings = timings.filter(
							(t) => t.categoryID === category.id,
						);
						return (
							<XAxisSection key={category.id}>
								{stageTimings.map((t) => {
									// If the stage has no timing data, we just want to render an empty row
									if (t.range === undefined) {
										return (
											<XAxisRow
												key={t.name}
												yAxisLabelId={encodeURIComponent(t.name)}
											/>
										);
									}

									const value = calcDuration(t.range);
									const offset = calcOffset(t.range, totalRange);

									return (
										<XAxisRow
											key={t.name}
											yAxisLabelId={encodeURIComponent(t.name)}
										>
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
													{t.error && (
														<ErrorSharp
															css={{
																fontSize: 18,
																color: "#F87171",
																marginRight: 4,
															}}
														/>
													)}
													<Blocks count={t.resources} />
												</ClickableBar>
											) : (
												<Bar scale={scale} value={value} offset={offset} />
											)}
											{formatTime(calcDuration(t.range))}
										</XAxisRow>
									);
								})}
							</XAxisSection>
						);
					})}
				</XAxis>
			</ChartContent>
		</Chart>
	);
};

const styles = {
	stageLabel: {
		display: "flex",
		alignItems: "center",
		gap: 2,
		justifyContent: "flex-end",
	},
	info: (theme) => ({
		width: 12,
		height: 12,
		color: theme.palette.text.secondary,
		cursor: "pointer",
	}),
} satisfies Record<string, Interpolation<Theme>>;
