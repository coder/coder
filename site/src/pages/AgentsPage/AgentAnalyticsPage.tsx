import { chatCostSummary } from "api/queries/chats";
import { useAuthContext } from "contexts/auth/AuthProvider";
import dayjs, { type Dayjs } from "dayjs";
import type { FC } from "react";
import { useQuery } from "react-query";
import { AgentAnalyticsPageView } from "./AgentAnalyticsPageView";
import { AgentPageHeader } from "./components/AgentPageHeader";

const createDateRange = (now?: Dayjs) => {
	const end = now ?? dayjs();
	const start = end.subtract(30, "day");
	return {
		startDate: start.toISOString(),
		endDate: end.toISOString(),
		rangeLabel: `${start.format("MMM D")} – ${end.format("MMM D, YYYY")}`,
	};
};

interface AgentAnalyticsPageProps {
	/** Override the current time for deterministic storybook snapshots. */
	now?: Dayjs;
}

const AgentAnalyticsPage: FC<AgentAnalyticsPageProps> = ({ now }) => {
	const { user } = useAuthContext();
	const dateRange = createDateRange(now);

	const summaryQuery = useQuery({
		...chatCostSummary(user?.id ?? "me", {
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
		}),
		enabled: Boolean(user?.id),
	});

	return (
		<>
			<AgentPageHeader />
			<AgentAnalyticsPageView
				summary={summaryQuery.data}
				isLoading={summaryQuery.isLoading}
				error={summaryQuery.error}
				onRetry={() => void summaryQuery.refetch()}
				rangeLabel={dateRange.rangeLabel}
			/>
		</>
	);
};

export default AgentAnalyticsPage;
