import { useState } from "react";
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

type ResourceTiming = {
	name: string;
	source: string;
	action: string;
	range: TimeRange;
};

type ResourcesChartProps = {
	stage: Stage;
	timings: ResourceTiming[];
	onBack: () => void;
};

export const ResourcesChart: React.FC<ResourcesChartProps> = ({
	stage,
	timings,
	onBack,
}) => {
	const generalTiming = mergeTimeRanges(timings.map((t) => t.range));
	const totalTime = calcDuration(generalTiming);
	const [ticks, scale] = makeTicks(totalTime);
	const [filter, setFilter] = useState("");
	const visibleTimings = timings.filter(
		// Stage boundaries are also included
		(t) =>
			(!isCoderResource(t.name) || isStageBoundary(t.name)) &&
			t.name.includes(filter),
	);
	const theme = useTheme();
	const legendsByAction = getLegendsByAction(theme);
	const visibleLegends = [...new Set(visibleTimings.map((t) => t.action))].map(
		(a) => legendsByAction[a] ?? { label: a },
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
				<ChartTable caption={`Resources in ${stage.name} stage`} ticks={ticks}>
					<ChartTableSection label={`${stage.name} stage`} ticks={ticks}>
						{visibleTimings.map((t) => {
							const duration = calcDuration(t.range);
							const legend = legendsByAction[t.action] ?? { label: t.action };
							const label = isStageBoundary(t.name)
								? "total stage duration"
								: t.name;

							return (
								<ChartTableRow key={t.name} label={label}>
									<Tooltip>
										<TooltipTrigger asChild>
											<FocusableBar
												aria-label={`${label}: ${legend.label}`}
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
											{label}
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

export const isStageBoundary = (resource: string) => {
	return resource.startsWith("coder_stage_");
};

export const isCoderResource = (resource: string) => {
	return (
		resource.startsWith("data.coder") ||
		resource.startsWith("module.coder") ||
		resource.startsWith("coder_")
	);
};

// TODO: We should probably strongly type the action attribute on
// ProvisionerTiming to catch missing actions in the record. As a "workaround"
// for now, we are using undefined since we don't have noUncheckedIndexedAccess
// enabled.
function getLegendsByAction(
	theme: Theme,
): Record<string, ChartLegend | undefined> {
	return {
		"state refresh": {
			label: "state refresh",
		},
		provision: {
			label: "provision",
		},
		create: {
			label: "create",
			colors: {
				fill: theme.roles.success.background,
				stroke: theme.roles.success.outline,
			},
		},
		delete: {
			label: "delete",
			colors: {
				fill: theme.roles.warning.background,
				stroke: theme.roles.warning.outline,
			},
		},
		read: {
			label: "read",
			colors: {
				fill: theme.roles.active.background,
				stroke: theme.roles.active.outline,
			},
		},
	};
}
