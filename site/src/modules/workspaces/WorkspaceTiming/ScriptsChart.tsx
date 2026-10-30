import { useState } from "react";
import type { WorkspaceAgentScriptStatus } from "#/api/typesGenerated";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { Theme } from "#/theme";
import { useTheme } from "#/theme/context";
import { FocusableBar } from "./Chart/Bar";
import {
	Chart,
	ChartBreadcrumbs,
	ChartContent,
	type ChartLegend,
	ChartLegends,
	ChartSearch,
	ChartToolbar,
} from "./Chart/Chart";
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
import type { Stage } from "./StagesChart";

type ScriptTiming = {
	name: string;
	status: WorkspaceAgentScriptStatus;
	exitCode: number;
	range: TimeRange;
};

type ScriptsChartProps = {
	stage: Stage;
	timings: ScriptTiming[];
	onBack: () => void;
};

export const ScriptsChart: React.FC<ScriptsChartProps> = ({
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
				<ChartTable caption={`Scripts in ${stage.name} stage`} ticks={ticks}>
					<ChartTableSection label={`${stage.name} stage`} ticks={ticks}>
						{visibleTimings.map((t) => {
							const duration = calcDuration(t.range);
							const legend = legendsByStatus[t.status];

							return (
								<ChartTableRow key={t.name} label={t.name}>
									<Tooltip>
										<TooltipTrigger asChild>
											<FocusableBar
												aria-label={`${t.name}: ${legend.label}`}
												value={duration}
												offset={calcOffset(t.range, generalTiming)}
												scale={scale}
												colors={legend.colors}
											/>
										</TooltipTrigger>
										<TooltipContent
											side="bottom"
											className="border-surface-quaternary text-content-primary"
										>
											{tooltipContent(t)}
										</TooltipContent>
									</Tooltip>

									{formatTime(duration)}
								</ChartTableRow>
							);
						})}
					</ChartTableSection>
				</ChartTable>
			</ChartContent>
		</Chart>
	);
};

// The runner reports 255 when it has no exit code, so a timeout or an open
// pipe must not be shown as an exit code.
function tooltipContent(t: ScriptTiming): React.ReactNode {
	switch (t.status) {
		case "timed_out":
			return "Script timed out.";
		case "pipes_left_open":
			return "Script exited without closing its output pipes.";
		case "skipped":
			return "Script was skipped because a script it depends on failed, timed out, or was skipped.";
		case "canceled":
			return "Script was canceled because the agent shut down before it finished.";
		default:
			return (
				<>
					Script exited with <strong>code {t.exitCode}</strong>
				</>
			);
	}
}

// Keyed by the union so a status without a legend fails the type check
// instead of throwing on .colors at runtime.
function getLegendsByStatus(
	theme: Theme,
): Record<WorkspaceAgentScriptStatus, ChartLegend> {
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
		timed_out: {
			label: "timed out",
			colors: {
				fill: theme.roles.warning.background,
				stroke: theme.roles.warning.outline,
			},
		},
		pipes_left_open: {
			label: "output pipes left open",
			colors: {
				fill: theme.roles.warning.background,
				stroke: theme.roles.warning.outline,
			},
		},
		skipped: {
			label: "skipped",
			colors: {
				fill: theme.roles.inactive.background,
				stroke: theme.roles.inactive.outline,
			},
		},
		// The notice role rather than info: info and inactive share the same
		// fill in both themes, so canceled and skipped bars would look alike.
		canceled: {
			label: "canceled",
			colors: {
				fill: theme.roles.notice.background,
				stroke: theme.roles.notice.outline,
			},
		},
	};
}
