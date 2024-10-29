import { css } from "@emotion/css";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import { type FC, useState } from "react";
import { Link } from "react-router-dom";
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
import type { StageCategory } from "./StagesChart";

type ResourceTiming = {
	name: string;
	source: string;
	action: string;
	range: TimeRange;
};

export type ResourcesChartProps = {
	category: StageCategory;
	stage: string;
	timings: ResourceTiming[];
	onBack: () => void;
};

export const ResourcesChart: FC<ResourcesChartProps> = ({
	category,
	stage,
	timings,
	onBack,
}) => {
	const generalTiming = mergeTimeRanges(timings.map((t) => t.range));
	const totalTime = calcDuration(generalTiming);
	const [ticks, scale] = makeTicks(totalTime);
	const [filter, setFilter] = useState("");
	const visibleTimings = timings.filter(
		(t) => !isCoderResource(t.name) && t.name.includes(filter),
	);
	const theme = useTheme();
	const legendsByAction = getLegendsByAction(theme);
	const visibleLegends = [...new Set(visibleTimings.map((t) => t.action))].map(
		(a) => legendsByAction[a],
	);

	return (
		<Chart>
			<ChartToolbar>
				<ChartBreadcrumbs
					breadcrumbs={[
						{
							label: category.name,
							onClick: onBack,
						},
						{
							label: stage,
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
						<YAxisHeader>{stage} stage</YAxisHeader>
						<YAxisLabels>
							{visibleTimings.map((t) => (
								<YAxisLabel key={t.name} id={encodeURIComponent(t.name)}>
									{t.name}
								</YAxisLabel>
							))}
						</YAxisLabels>
					</YAxisSection>
				</YAxis>

				<XAxis ticks={ticks} scale={scale}>
					<XAxisSection>
						{visibleTimings.map((t) => {
							const duration = calcDuration(t.range);

							return (
								<XAxisRow
									key={t.name}
									yAxisLabelId={encodeURIComponent(t.name)}
								>
									<Tooltip
										title={
											<>
												<TooltipTitle>{t.name}</TooltipTitle>
												<TooltipLink to="">view template</TooltipLink>
											</>
										}
									>
										<Bar
											value={duration}
											offset={calcOffset(t.range, generalTiming)}
											scale={scale}
											colors={legendsByAction[t.action].colors}
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

export const isCoderResource = (resource: string) => {
	return (
		resource.startsWith("data.coder") ||
		resource.startsWith("module.coder") ||
		resource.startsWith("coder_")
	);
};

function getLegendsByAction(theme: Theme): Record<string, ChartLegend> {
	return {
		"state refresh": {
			label: "state refresh",
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
