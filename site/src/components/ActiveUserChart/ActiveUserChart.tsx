import "chartjs-adapter-date-fns";
import { useTheme } from "@emotion/react";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import {
	CategoryScale,
	Chart as ChartJS,
	type ChartOptions,
	Filler,
	Legend,
	LineElement,
	LinearScale,
	PointElement,
	TimeScale,
	Title,
	Tooltip,
	defaults,
} from "chart.js";
import annotationPlugin from "chartjs-plugin-annotation";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import dayjs from "dayjs";
import type { FC } from "react";
import { Line } from "react-chartjs-2";

ChartJS.register(
	CategoryScale,
	LinearScale,
	TimeScale,
	LineElement,
	PointElement,
	Filler,
	Title,
	Tooltip,
	Legend,
	annotationPlugin,
);

export interface DataSeries {
	label?: string;
	data: readonly { date: string; amount: number }[];
	color?: string; // Optional custom color
}

export interface ActiveUserChartProps {
	series: DataSeries[];
	userLimit?: number;
	interval: "day" | "week";
}

export const ActiveUserChart: FC<ActiveUserChartProps> = ({
	series,
	userLimit,
	interval,
}) => {
	const theme = useTheme();

	defaults.font.family = theme.typography.fontFamily as string;
	defaults.color = theme.palette.text.secondary;

	const options: ChartOptions<"line"> = {
		responsive: true,
		animation: false,
		interaction: {
			mode: "index",
		},
		plugins: {
			legend:
				series.length > 1
					? {
							display: false,
							position: "top" as const,
							labels: {
								usePointStyle: true,
								pointStyle: "line",
							},
						}
					: {
							display: false,
						},
			tooltip: {
				displayColors: false,
				callbacks: {
					title: (context) => {
						const date = new Date(context[0].parsed.x);
						return date.toLocaleDateString();
					},
				},
			},
			annotation: {
				annotations: [
					{
						type: "line",
						scaleID: "y",
						value: userLimit,
						borderColor: "white",
						borderWidth: 2,
						label: {
							content: "Active User limit",
							color: theme.palette.primary.contrastText,
							display: true,
							textStrokeWidth: 2,
							textStrokeColor: theme.palette.background.paper,
						},
					},
				],
			},
		},
		scales: {
			y: {
				grid: { color: theme.palette.divider },
				suggestedMin: 0,
				ticks: {
					precision: 0,
				},
				stacked: true,
			},
			x: {
				grid: { color: theme.palette.divider },
				ticks: {
					stepSize: series[0].data.length > 10 ? 2 : undefined,
				},
				type: "time",
				time: {
					unit: interval,
				},
			},
		},
		maintainAspectRatio: false,
	};

	return (
		<Line
			data-chromatic="ignore"
			data={{
				labels: series[0].data.map((val) =>
					dayjs(val.date).format("YYYY-MM-DD"),
				),
				datasets: series.map((s) => ({
					label: s.label,
					data: s.data.map((val) => val.amount),
					pointBackgroundColor: s.color || theme.roles.active.outline,
					pointBorderColor: s.color || theme.roles.active.outline,
					borderColor: s.color || theme.roles.active.outline,
				})),
			}}
			options={options}
		/>
	);
};

type ActiveUsersTitleProps = {
	interval: "day" | "week";
};

export const ActiveUsersTitle: FC<ActiveUsersTitleProps> = ({ interval }) => {
	return (
		<div css={{ display: "flex", alignItems: "center", gap: 8 }}>
			{interval === "day" ? "Daily" : "Weekly"} User Activity
			<HelpTooltip>
				<HelpTooltipTrigger size="small" />
				<HelpTooltipContent>
					<HelpTooltipTitle>
						How do we calculate user activity?
					</HelpTooltipTitle>
					<HelpTooltipText>
						When a connection is initiated to a user&apos;s workspace they are
						considered an active user. e.g. apps, web terminal, SSH. This is for
						measuring user activity and has no connection to license
						consumption.
					</HelpTooltipText>
				</HelpTooltipContent>
			</HelpTooltip>
		</div>
	);
};

export type UserStatusTitleProps = {
	interval: "day" | "week";
};

export const UserStatusTitle: FC<UserStatusTitleProps> = ({ interval }) => {
	return (
		<div css={{ display: "flex", alignItems: "center", gap: 8 }}>
			{interval === "day" ? "Daily" : "Weekly"} User Status
			<HelpTooltip>
				<HelpTooltipTrigger size="small" />
				<HelpTooltipContent>
					<HelpTooltipTitle>What are user statuses?</HelpTooltipTitle>
					<HelpTooltipText
						css={{ display: "flex", gap: 8, flexDirection: "column" }}
					>
						<span>
							Active users count towards your license consumption. Dormant or
							suspended users do not. Any user who has logged into the coder
							platform within the last 90 days is considered active.
						</span>
						<Button
							component="a"
							startIcon={<LaunchOutlined />}
							href="https://coder.com/docs/admin/users#user-status"
							target="_blank"
							rel="noreferrer"
						>
							Read the docs
						</Button>
					</HelpTooltipText>
				</HelpTooltipContent>
			</HelpTooltip>
		</div>
	);
};
