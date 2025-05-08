import type { Interpolation, Theme } from "@emotion/react";
import type { TimingStage } from "api/typesGenerated";
import { ErrorSharp, InfoOutlined } from "lucide-react";
import type { FC } from "react";
import { Bar, ClickableBar } from "./Chart/Bar";
import { Blocks } from "./Chart/Blocks";
import { Chart, ChartContent } from "./Chart/Chart";
import {
	Tooltip,
	type TooltipProps,
	TooltipShortDescription,
	TooltipTitle,
} from "./Chart/Tooltip";
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

export type Stage = {
	/**
	 * The name is used to identify the stage.
	 */
	name: TimingStage;
	/**
	 * The value to display in the stage label. This can differ from the stage
	 * name to provide more context or clarity.
	 */
	label: string;
	/**
	 * The section is used to group stages together.
	 */
	section: string;
	/**
	 * The tooltip is used to provide additional information about the stage.
	 */
	tooltip: Omit<TooltipProps, "children">;
};

type StageTiming = {
	stage: Stage;
	/**
	 * Represents the number of resources included in this stage that can be
	 * inspected. This value is used to display individual blocks within the bar,
	 * indicating that the stage consists of multiple resource time blocks.
	 */
	visibleResources: number;
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
	onSelectStage: (stage: Stage) => void;
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
	const sections = Array.from(new Set(timings.map((t) => t.stage.section)));

	return (
		<Chart>
			<ChartContent>
				<YAxis>
					{sections.map((section) => {
						const stages = timings
							.filter((t) => t.stage.section === section)
							.map((t) => t.stage);

						return (
							<YAxisSection key={section}>
								<YAxisHeader>{section}</YAxisHeader>
								<YAxisLabels>
									{stages.map((stage) => (
										<YAxisLabel
											key={stage.name}
											id={encodeURIComponent(stage.name)}
										>
											<span css={styles.stageLabel}>
												{stage.label}
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
					{sections.map((section) => {
						const stageTimings = timings.filter(
							(t) => t.stage.section === section,
						);
						return (
							<XAxisSection key={section}>
								{stageTimings.map((t) => {
									// If the stage has no timing data, we just want to render an empty row
									if (t.range === undefined) {
										return (
											<XAxisRow
												key={t.stage.name}
												yAxisLabelId={encodeURIComponent(t.stage.name)}
											/>
										);
									}

									const value = calcDuration(t.range);
									const offset = calcOffset(t.range, totalRange);

									return (
										<XAxisRow
											key={t.stage.name}
											yAxisLabelId={encodeURIComponent(t.stage.name)}
										>
											{/** We only want to expand stages with more than one resource */}
											{t.visibleResources > 1 ? (
												<ClickableBar
													aria-label={`View ${t.stage.label} details`}
													scale={scale}
													value={value}
													offset={offset}
													onClick={() => {
														onSelectStage(t.stage);
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
													<Blocks count={t.visibleResources} />
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
	stageDescription: {
		maxWidth: 300,
	},
	info: (theme) => ({
		width: 12,
		height: 12,
		color: theme.palette.text.secondary,
		cursor: "pointer",
	}),
} satisfies Record<string, Interpolation<Theme>>;

export const provisioningStages: Stage[] = [
	{
		name: "init",
		label: "init",
		section: "provisioning",
		tooltip: {
			title: (
				<>
					<TooltipTitle>Terraform initialization</TooltipTitle>
					<TooltipShortDescription>
						Download providers & modules.
					</TooltipShortDescription>
				</>
			),
		},
	},
	{
		name: "plan",
		label: "plan",
		section: "provisioning",
		tooltip: {
			title: (
				<>
					<TooltipTitle>Terraform plan</TooltipTitle>
					<TooltipShortDescription>
						Compare state of desired vs actual resources and compute changes to
						be made.
					</TooltipShortDescription>
				</>
			),
		},
	},
	{
		name: "graph",
		label: "graph",
		section: "provisioning",
		tooltip: {
			title: (
				<>
					<TooltipTitle>Terraform graph</TooltipTitle>
					<TooltipShortDescription>
						List all resources in plan, used to update coderd database.
					</TooltipShortDescription>
				</>
			),
		},
	},
	{
		name: "apply",
		label: "apply",
		section: "provisioning",
		tooltip: {
			title: (
				<>
					<TooltipTitle>Terraform apply</TooltipTitle>
					<TooltipShortDescription>
						Execute Terraform plan to create/modify/delete resources into
						desired states.
					</TooltipShortDescription>
				</>
			),
		},
	},
];

export const agentStages = (section: string): Stage[] => {
	return [
		{
			name: "connect",
			label: "connect",
			section,
			tooltip: {
				title: (
					<>
						<TooltipTitle>Connect</TooltipTitle>
						<TooltipShortDescription>
							Establish an RPC connection with the control plane.
						</TooltipShortDescription>
					</>
				),
			},
		},
		{
			name: "start",
			label: "run startup scripts",
			section,
			tooltip: {
				title: (
					<>
						<TooltipTitle>Run startup scripts</TooltipTitle>
						<TooltipShortDescription>
							Execute each agent startup script.
						</TooltipShortDescription>
					</>
				),
			},
		},
	];
};
