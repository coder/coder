import dayjs, { type Dayjs } from "dayjs";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { prInsights } from "#/api/queries/chats";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { InsightsContent } from "./components/InsightsContent";
import type { PRInsightsTimeRange } from "./components/PRInsightsView";

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

const AgentSettingsInsightsPage: FC = () => {
	const { permissions } = useAuthenticated();

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

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<InsightsContent
				data={data}
				isLoading={isLoading}
				error={error}
				timeRange={selection.timeRange}
				onTimeRangeChange={handleTimeRangeChange}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsInsightsPage;
