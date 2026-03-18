import type { PRInsightsResponse } from "api/typesGenerated";
import { Spinner } from "components/Spinner/Spinner";
import dayjs from "dayjs";
import { type FC, useCallback, useMemo, useState } from "react";
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

async function fetchPRInsights(
	startDate: string,
	endDate: string,
): Promise<PRInsightsResponse> {
	const params = new URLSearchParams({
		start_date: startDate,
		end_date: endDate,
	});
	const resp = await fetch(
		`/api/v2/chats/insights/pull-requests?${params.toString()}`,
	);
	if (!resp.ok) {
		throw new Error(`Failed to fetch PR insights: ${resp.statusText}`);
	}
	return resp.json();
}

export const InsightsContent: FC = () => {
	const [timeRange, setTimeRange] = useState<PRInsightsTimeRange>("30d");
	const dates = useMemo(() => timeRangeToDates(timeRange), [timeRange]);

	const { data, isLoading, error } = useQuery({
		queryKey: ["prInsights", dates.start_date, dates.end_date],
		queryFn: () => fetchPRInsights(dates.start_date, dates.end_date),
	});

	const handleTimeRangeChange = useCallback(
		(range: PRInsightsTimeRange) => setTimeRange(range),
		[],
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
			timeRange={timeRange}
			onTimeRangeChange={handleTimeRangeChange}
		/>
	);
};
