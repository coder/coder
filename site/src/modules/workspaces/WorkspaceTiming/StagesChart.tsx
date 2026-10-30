import { CircleAlertIcon, InfoIcon } from "lucide-react";
import type { TimingStage } from "#/api/typesGenerated";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { Bar, ClickableBar } from "./Chart/Bar";
import { Blocks } from "./Chart/Blocks";
import { Chart, ChartContent } from "./Chart/Chart";
import {
	ChartTable,
	ChartTableRow,
	ChartTableSection,
} from "./Chart/ChartTable";
import {
	calcDuration,
	calcOffset,
	formatTime,
	makeTicks,
	mergeTimeRanges,
	type TimeRange,
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
	 * The agent ID for agent-related stages. Used to filter timings correctly
	 * when multiple agents exist.
	 */
	agentId?: string;
	/**
	 * The tooltip is used to provide additional information about the stage.
	 */
	tooltip: {
		heading: string;
		description: string;
	};
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

type StagesChartProps = {
	timings: StageTiming[];
	onSelectStage: (stage: Stage) => void;
};

export const StagesChart: React.FC<StagesChartProps> = ({
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
				<ChartTable caption="Build timeline stages" ticks={ticks}>
					{sections.map((section, i) => {
						const stageTimings = timings.filter(
							(t) => t.stage.section === section,
						);
						return (
							<ChartTableSection
								key={section}
								label={section}
								ticks={i === 0 ? ticks : undefined}
							>
								{stageTimings.map((t) => (
									<ChartTableRow
										key={t.stage.name}
										label={<StageLabel stage={t.stage} />}
									>
										{t.range !== undefined && (
											<StageBar
												timing={t}
												range={t.range}
												totalRange={totalRange}
												scale={scale}
												onSelectStage={onSelectStage}
											/>
										)}
									</ChartTableRow>
								))}
							</ChartTableSection>
						);
					})}
				</ChartTable>
			</ChartContent>
		</Chart>
	);
};

type StageLabelProps = {
	stage: Stage;
};

const StageLabel: React.FC<StageLabelProps> = ({ stage }) => {
	return (
		<span className="flex items-center justify-end gap-0.5">
			{stage.label}
			<Tooltip>
				<TooltipTrigger asChild>
					<InfoIcon className="size-icon-xs cursor-pointer text-content-secondary" />
				</TooltipTrigger>
				<TooltipContent
					side="bottom"
					className="flex flex-col gap-1.5 max-w-xs border-surface-quaternary"
				>
					<p className="m-0 text-content-primary">{stage.tooltip.heading}</p>
					<p className="m-0">{stage.tooltip.description}</p>
				</TooltipContent>
			</Tooltip>
		</span>
	);
};

type StageBarProps = {
	timing: StageTiming;
	range: TimeRange;
	totalRange: TimeRange;
	scale: number;
	onSelectStage: (stage: Stage) => void;
};

const StageBar: React.FC<StageBarProps> = ({
	timing,
	range,
	totalRange,
	scale,
	onSelectStage,
}) => {
	const value = calcDuration(range);
	const offset = calcOffset(range, totalRange);
	const validDuration = value > 0 && !Number.isNaN(value);

	return (
		<>
			{timing.visibleResources > 1 ? (
				<ClickableBar
					aria-label={`View ${timing.stage.label} details`}
					scale={scale}
					value={value}
					offset={offset}
					onClick={() => {
						onSelectStage(timing.stage);
					}}
				>
					{timing.error && (
						<CircleAlertIcon className="size-icon-sm text-[#F87171] mr-1" />
					)}
					<Blocks count={timing.visibleResources} />
				</ClickableBar>
			) : (
				<Bar aria-hidden scale={scale} value={value} offset={offset} />
			)}
			{validDuration ? (
				<span>{formatTime(value)}</span>
			) : (
				<span className="text-content-destructive">Invalid</span>
			)}
		</>
	);
};

export const provisioningStages: Stage[] = [
	{
		name: "init",
		label: "init",
		section: "provisioning",
		tooltip: {
			heading: "Terraform initialization",
			description: "Download providers & modules.",
		},
	},
	{
		name: "plan",
		label: "plan",
		section: "provisioning",
		tooltip: {
			heading: "Terraform plan",
			description:
				"Compare state of desired vs actual resources and compute changes to be made.",
		},
	},
	{
		name: "apply",
		label: "apply",
		section: "provisioning",
		tooltip: {
			heading: "Terraform apply",
			description:
				"Execute Terraform plan to create/modify/delete resources into desired states.",
		},
	},
	{
		name: "graph",
		label: "graph",
		section: "provisioning",
		tooltip: {
			heading: "Terraform graph",
			description:
				"List all resources in plan, used to update coderd database.",
		},
	},
];

export const agentStages = (section: string, agentId: string): Stage[] => {
	return [
		{
			name: "connect",
			label: "connect",
			section,
			agentId,
			tooltip: {
				heading: "Connect",
				description: "Establish an RPC connection with the control plane.",
			},
		},
		{
			name: "start",
			label: "run startup scripts",
			section,
			agentId,
			tooltip: {
				heading: "Run startup scripts",
				description: "Execute each agent startup script.",
			},
		},
	];
};
