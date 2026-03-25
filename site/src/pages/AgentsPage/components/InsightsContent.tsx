import dayjs, { type Dayjs } from "dayjs";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { prInsights } from "#/api/queries/chats";
import { Spinner } from "#/components/Spinner/Spinner";
import { type PRInsightsTimeRange, PRInsightsView } from "./PRInsightsView";

type TimeRangeSelection = {
	timeRange: PRInsightsTimeRange;
	anchor: Dayjs;
};

function timeRangeToDates(range: PRInsightsTimeRange, anchor: Dayjs) {
	const days = Number.parseInt(range, 10);
	const start = anchor.subtract(days, "day");
	return {
		start_date: start.toISOString(),
		end_date: anchor.toISOString(),
	};
}

export const InsightsContent: FC = () => {
	const [selection, setSelection] = useState<TimeRangeSelection>(() => ({
		timeRange: "30d",
		anchor: dayjs(),
	}));
	const dates = timeRangeToDates(selection.timeRange, selection.anchor);

	const { data, isLoading, error } = useQuery(prInsights(dates));

	const handleTimeRangeChange = (timeRange: PRInsightsTimeRange) =>
		setSelection((current) =>
			current.timeRange === timeRange
				? current
				: {
						timeRange,
						anchor: dayjs(),
					},
		);

	if (isLoading) {
		return (
			<div className="flex min-h-[400px] items-center justify-center">
				<Spinner size="lg" loading />
			</div>
		);
	}

	if (error) {
		return (
			<div className="flex min-h-[400px] items-center justify-center">
				<p className="text-sm text-content-secondary">
					Failed to load analytics data.
				</p>
			</div>
		);
	}

	if (!data) {
		return null;
	}

	return (
		<PRInsightsView
			data={data}
			timeRange={selection.timeRange}
			onTimeRangeChange={handleTimeRangeChange}
		/>
	);
};
