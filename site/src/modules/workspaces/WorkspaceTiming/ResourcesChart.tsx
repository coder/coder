import {
	XAxis,
	XAxisRow,
	XAxisRows,
	XAxisSections,
	XAxisMinWidth,
} from "./Chart/XAxis";
import { useState, type FC } from "react";
import {
	YAxis,
	YAxisCaption,
	YAxisLabel,
	YAxisLabels,
	YAxisSection,
} from "./Chart/YAxis";
import { Bar } from "./Chart/Bar";
import {
	calcDuration,
	calcOffset,
	combineTimings,
	formatTime,
	makeTicks,
	type BaseTiming,
} from "./Chart/utils";
import {
	Chart,
	ChartBreadcrumbs,
	ChartContent,
	type ChartLegend,
	ChartLegends,
	ChartSearch,
	ChartToolbar,
} from "./Chart/Chart";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Tooltip, { type TooltipProps } from "@mui/material/Tooltip";
import { css } from "@emotion/css";
import { Link } from "react-router-dom";
import OpenInNewOutlined from "@mui/icons-material/OpenInNewOutlined";

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

type ResourceTiming = BaseTiming & {
	name: string;
	source: string;
	action: string;
};

export type ResourcesChartProps = {
	category: string;
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
	const generalTiming = combineTimings(timings);
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
							label: category,
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
								<YAxisLabel key={t.name} id={t.name}>
									{t.name}
								</YAxisLabel>
							))}
						</YAxisLabels>
					</YAxisSection>
				</YAxis>

				<XAxis ticks={ticks} scale={scale}>
					<XAxisSections>
						<XAxisRows>
							{visibleTimings.map((t) => {
								return (
									<XAxisRow key={t.name} yAxisLabelId={t.name}>
										<ResourceTooltip timing={t}>
											<Bar
												value={calcDuration(t)}
												offset={calcOffset(t, generalTiming)}
												scale={scale}
												colors={legendsByAction[t.action].colors}
											/>
										</ResourceTooltip>
										{formatTime(calcDuration(t), scale)}
									</XAxisRow>
								);
							})}
						</XAxisRows>
					</XAxisSections>
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
