import { css } from "@emotion/css";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import InfoOutlined from "@mui/icons-material/InfoOutlined";
import Tooltip, { type TooltipProps } from "@mui/material/Tooltip";
import type { FC, PropsWithChildren } from "react";
import { Bar, ClickableBar } from "./Chart/Bar";
import { BarBlocks } from "./Chart/BarBlocks";
import { Chart, ChartContent } from "./Chart/Chart";
import {
	XAxis,
	XAxisMinWidth,
	XAxisRow,
	XAxisRows,
	XAxisSections,
} from "./Chart/XAxis";
import {
	YAxis,
	YAxisCaption,
	YAxisLabel,
	YAxisLabels,
	YAxisSection,
} from "./Chart/YAxis";
import {
	type BaseTiming,
	calcDuration,
	calcOffset,
	combineTimings,
	formatTime,
	makeTicks,
} from "./Chart/utils";

// TODO: Add "workspace boot" when scripting timings are done.
const stageCategories = ["provisioning"] as const;

type StageCategory = (typeof stageCategories)[number];

type Stage = {
	name: string;
	category: StageCategory;
	tooltip: { title: string; description: string };
};

// TODO: Export provisioning stages from the BE to the generated types.
export const stages: Stage[] = [
	{
		name: "init",
		category: "provisioning",
		tooltip: {
			title: "Terraform initialization",
			description: "Download providers & modules.",
		},
	},
	{
		name: "plan",
		category: "provisioning",
		tooltip: {
			title: "Terraform plan",
			description:
				"Compare state of desired vs actual resources and compute changes to be made.",
		},
	},
	{
		name: "graph",
		category: "provisioning",
		tooltip: {
			title: "Terraform graph",
			description:
				"List all resources in plan, used to update coderd database.",
		},
	},
	{
		name: "apply",
		category: "provisioning",
		tooltip: {
			title: "Terraform apply",
			description:
				"Execute terraform plan to create/modify/delete resources into desired states.",
		},
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
												{formatTime(calcDuration(t))}
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
