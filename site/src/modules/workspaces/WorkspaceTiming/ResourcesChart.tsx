import { type Theme, useTheme } from "@emotion/react";
import { type FC, useState } from "react";
import { Bar } from "./Chart/Bar";
import {
	Chart,
	ChartBreadcrumbs,
	ChartContent,
	type ChartLegend,
	ChartLegends,
	ChartSearch,
	ChartToolbar,
} from "./Chart/Chart";
import { Tooltip, TooltipLink, TooltipTitle } from "./Chart/Tooltip";
import {
	calcDuration,
	calcOffset,
	formatTime,
	makeTicks,
	mergeTimeRanges,
	type TimeRange,
} from "./Chart/utils";
import { XAxis, XAxisRow, XAxisSection } from "./Chart/XAxis";
import {
	YAxis,
	YAxisHeader,
	YAxisLabel,
	YAxisLabels,
	YAxisSection,
} from "./Chart/YAxis";
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

export const ResourcesChart: FC<ResourcesChartProps> = ({
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
				<YAxis>
					<YAxisSection>
						<YAxisHeader>{stage.name} stage</YAxisHeader>
						<YAxisLabels>
							{visibleTimings.map((t) => {
								const label = isStageBoundary(t.name)
									? "total stage duration"
									: t.name;
								return (
									<YAxisLabel key={label} id={encodeURIComponent(t.name)}>
										{label}
									</YAxisLabel>
								);
							})}
						</YAxisLabels>
					</YAxisSection>
				</YAxis>

				<XAxis ticks={ticks} scale={scale}>
					<XAxisSection>
						{visibleTimings.map((t) => {
							const stageBoundary = isStageBoundary(t.name);
							const duration = calcDuration(t.range);
							const legend = legendsByAction[t.action] ?? { label: t.action };
							const label = stageBoundary ? "total stage duration" : t.name;

							return (
								<XAxisRow
									key={t.name}
									yAxisLabelId={encodeURIComponent(t.name)}
								>
									<Tooltip
										title={
											<>
												<TooltipTitle>{label}</TooltipTitle>
												{/* Stage boundaries should not have these links */}
												{!stageBoundary && (
													<TooltipLink to="">view template</TooltipLink>
												)}
											</>
										}
									>
										<Bar
											value={duration}
											offset={calcOffset(t.range, generalTiming)}
											scale={scale}
											colors={legend.colors}
										/>
									</Tooltip>
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
