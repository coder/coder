import { css } from "@emotion/css";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";
import Tooltip, { type TooltipProps } from "@mui/material/Tooltip";
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
import type { StageCategory } from "./StagesChart";

const legendsByAction: Record<string, ChartLegend> = {
	"state refresh": {
		label: "state refresh",
	},
	create: {
		label: "create",
		colors: {
			fill: "#022C22",
			stroke: "#BBF7D0",
		},
	},
	delete: {
		label: "delete",
		colors: {
			fill: "#422006",
			stroke: "#FDBA74",
		},
	},
	read: {
		label: "read",
		colors: {
			fill: "#082F49",
			stroke: "#38BDF8",
		},
	},
};

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
						<YAxisCaption>{stage} stage</YAxisCaption>
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
									<ResourceTooltip timing={t}>
										<Bar
											value={duration}
											offset={calcOffset(t.range, generalTiming)}
											scale={scale}
											colors={legendsByAction[t.action].colors}
										/>
									</ResourceTooltip>
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

const isCoderResource = (resource: string) => {
	return (
		resource.startsWith("data.coder") ||
		resource.startsWith("module.coder") ||
		resource.startsWith("coder_")
	);
};

type ResourceTooltipProps = Omit<TooltipProps, "title"> & {
	timing: ResourceTiming;
};

const ResourceTooltip: FC<ResourceTooltipProps> = ({ timing, ...props }) => {
	const theme = useTheme();

	return (
		<Tooltip
			{...props}
			placement="top-start"
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
					<span>{timing.source}</span>
					<span css={styles.tooltipResource}>{timing.name}</span>
					<Link to="" css={styles.tooltipLink}>
						<OpenInNewOutlined />
						view template
					</Link>
				</div>
			}
		/>
	);
};

const styles = {
	tooltipTitle: (theme) => ({
		display: "flex",
		flexDirection: "column",
		fontWeight: 500,
		fontSize: 12,
		color: theme.palette.text.secondary,
	}),
	tooltipResource: (theme) => ({
		color: theme.palette.text.primary,
		fontWeight: 600,
		marginTop: 4,
		display: "block",
		maxWidth: "100%",
		overflow: "hidden",
		textOverflow: "ellipsis",
		whiteSpace: "nowrap",
	}),
	tooltipLink: (theme) => ({
		color: "inherit",
		textDecoration: "none",
		display: "flex",
		alignItems: "center",
		gap: 4,
		marginTop: 8,

		"&:hover": {
			color: theme.palette.text.primary,
		},

		"& svg": {
			width: 12,
			height: 12,
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
