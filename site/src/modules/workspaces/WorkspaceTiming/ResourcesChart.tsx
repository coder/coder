import { useState } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { FocusableBar } from "./Chart/Bar";
import {
	Chart,
	ChartBreadcrumbs,
	type ChartCategory,
	ChartCategoryLabel,
	ChartContent,
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
			</ChartToolbar>
			<ChartContent>
				<ChartTable caption={`Resources in ${stage.name} stage`} ticks={ticks}>
					<ChartTableSection label={`${stage.name} stage`} ticks={ticks}>
						{visibleTimings.map((t) => {
							const duration = calcDuration(t.range);
							const category = categoriesByAction[t.action] ?? {
								label: t.action,
							};
							const label = isStageBoundary(t.name)
								? "total stage duration"
								: t.name;

							return (
								<ChartTableRow key={t.name} label={label}>
									<Tooltip>
										<TooltipTrigger asChild>
											<FocusableBar
												aria-label={`${label}: ${category.label}`}
												value={duration}
												offset={calcOffset(t.range, generalTiming)}
												scale={scale}
												className={category.className}
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
									<ChartCategoryLabel label={category.label} />
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
const categoriesByAction: Record<string, ChartCategory | undefined> = {
	"state refresh": {
		label: "state refresh",
	},
	provision: {
		label: "provision",
	},
	create: {
		label: "create",
		className: "bg-chart-success border-chart-success-border",
	},
	delete: {
		label: "delete",
		className: "bg-chart-warning border-chart-warning-border",
	},
	read: {
		label: "read",
		className: "bg-chart-active border-chart-active-border",
	},
};
