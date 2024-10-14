import { css } from "@emotion/css";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import InfoOutlined from "@mui/icons-material/InfoOutlined";
import Tooltip, { type TooltipProps } from "@mui/material/Tooltip";
import type { FC } from "react";
import { Bar, ClickableBar } from "./Chart/Bar";
import { BarBlocks } from "./Chart/BarBlocks";
import { Chart, ChartContent } from "./Chart/Chart";
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
	tooltip: { title: string; description: string };
};

export const stages: Stage[] = [
	{
		name: "init",
		categoryID: "provisioning",
		tooltip: {
			title: "Terraform initialization",
			description: "Download providers & modules.",
		},
	},
	{
		name: "plan",
		categoryID: "provisioning",
		tooltip: {
			title: "Terraform plan",
			description:
				"Compare state of desired vs actual resources and compute changes to be made.",
		},
	},
	{
		name: "graph",
		categoryID: "provisioning",
		tooltip: {
			title: "Terraform graph",
			description:
				"List all resources in plan, used to update coderd database.",
		},
	},
	{
		name: "apply",
		categoryID: "provisioning",
		tooltip: {
			title: "Terraform apply",
			description:
				"Execute terraform plan to create/modify/delete resources into desired states.",
		},
	},
	{
		name: "start",
		categoryID: "workspaceBoot",
		tooltip: {
			title: "Start",
			description: "Scripts executed when the agent is starting.",
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
								<YAxisCaption>{c.name}</YAxisCaption>
								<YAxisLabels>
									{stagesInCategory.map((stage) => (
										<YAxisLabel
											key={stage.name}
											id={encodeURIComponent(stage.name)}
										>
											<span css={styles.stageLabel}>
												{stage.name}
												<StageInfoTooltip {...stage.tooltip}>
													<InfoOutlined css={styles.info} />
												</StageInfoTooltip>
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
													<BarBlocks count={t.resources} />
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

type StageInfoTooltipProps = TooltipProps & {
	title: string;
	description: string;
};

const StageInfoTooltip: FC<StageInfoTooltipProps> = ({
	title,
	description,
	children,
}) => {
	const theme = useTheme();

	return (
		<Tooltip
			classes={{
				tooltip: css({
					backgroundColor: theme.palette.background.default,
					border: `1px solid ${theme.palette.divider}`,
					width: 220,
					borderRadius: 8,
				}),
			}}
			title={
				<div css={styles.tooltipTitle}>
					<span css={styles.infoStageName}>{title}</span>
					<span>{description}</span>
				</div>
			}
		>
			{children}
		</Tooltip>
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
	tooltipTitle: (theme) => ({
		display: "flex",
		flexDirection: "column",
		fontWeight: 500,
		fontSize: 12,
		color: theme.palette.text.secondary,
		gap: 4,
	}),
	infoStageName: (theme) => ({
		color: theme.palette.text.primary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
