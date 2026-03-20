import { prInsights } from "api/queries/chats";
import { Spinner } from "components/Spinner/Spinner";
import dayjs from "dayjs";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { type PRInsightsTimeRange, PRInsightsView } from "./PRInsightsView";

function timeRangeToDates(range: PRInsightsTimeRange) {
	const end = dayjs();
	const days = Number.parseInt(range, 10);
	const start = end.subtract(days, "day");
	return {
		start_date: start.toISOString(),
		end_date: end.toISOString(),
	};
}

export const InsightsContent: FC = () => {
	const [timeRange, setTimeRange] = useState<PRInsightsTimeRange>("30d");
	const dates = timeRangeToDates(timeRange);

	const { data, isLoading, error } = useQuery(prInsights(dates));

	const handleTimeRangeChange = (range: PRInsightsTimeRange) =>
		setTimeRange(range);

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
			timeRange={timeRange}
			onTimeRangeChange={handleTimeRangeChange}
		/>
	);
};
