import "chartjs-adapter-date-fns";
import { useTheme } from "@emotion/react";
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
);

export interface ActiveUserChartProps {
	data: readonly { date: string; amount: number }[];
	interval: "day" | "week";
}

export const ActiveUserChart: FC<ActiveUserChartProps> = ({
	data,
	interval,
}) => {
	const theme = useTheme();

	const labels = data.map((val) => dayjs(val.date).format("YYYY-MM-DD"));
	const chartData = data.map((val) => val.amount);

	defaults.font.family = theme.typography.fontFamily as string;
	defaults.color = theme.palette.text.secondary;

	const options: ChartOptions<"line"> = {
		responsive: true,
		animation: false,
		plugins: {
			legend: {
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
		},
		scales: {
			y: {
				grid: { color: theme.palette.divider },
				suggestedMin: 0,
				ticks: {
					precision: 0,
				},
			},
			x: {
				grid: { color: theme.palette.divider },
				ticks: {
					stepSize: data.length > 10 ? 2 : undefined,
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
				labels: labels,
				datasets: [
					{
						label: `${interval === "day" ? "Daily" : "Weekly"} Active Users`,
						data: chartData,
						pointBackgroundColor: theme.roles.active.outline,
						pointBorderColor: theme.roles.active.outline,
						borderColor: theme.roles.active.outline,
					},
				],
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
			{interval === "day" ? "Daily" : "Weekly"} Active Users
			<HelpTooltip>
				<HelpTooltipTrigger size="small" />
				<HelpTooltipContent>
					<HelpTooltipTitle>How do we calculate active users?</HelpTooltipTitle>
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
