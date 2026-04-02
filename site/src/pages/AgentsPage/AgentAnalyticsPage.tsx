import dayjs, { type Dayjs } from "dayjs";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { chatCostSummary } from "#/api/queries/chats";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { useAuthContext } from "#/contexts/auth/AuthProvider";
import { AgentAnalyticsPageView } from "./AgentAnalyticsPageView";
import { AgentPageHeader } from "./components/AgentPageHeader";

const startDateSearchParam = "startDate";
const endDateSearchParam = "endDate";

const getDefaultDateRange = (now?: Dayjs): DateRangeValue => {
	const end = now ?? dayjs();
	return {
		startDate: end.subtract(30, "day").toDate(),
		endDate: end.toDate(),
	};
};

interface AgentAnalyticsPageProps {
	/** Override the current time for deterministic storybook snapshots. */
	now?: Dayjs;
}

const AgentAnalyticsPage: FC<AgentAnalyticsPageProps> = ({ now }) => {
	const { user } = useAuthContext();

	const [searchParams, setSearchParams] = useSearchParams();
	const startDateParam = searchParams.get(startDateSearchParam)?.trim() ?? "";
	const endDateParam = searchParams.get(endDateSearchParam)?.trim() ?? "";
	const [defaultDateRange] = useState(() => getDefaultDateRange(now));
	let dateRange = defaultDateRange;
	let hasExplicitDateRange = false;

	if (startDateParam && endDateParam) {
		const parsedStartDate = new Date(startDateParam);
		const parsedEndDate = new Date(endDateParam);

		if (
			!Number.isNaN(parsedStartDate.getTime()) &&
			!Number.isNaN(parsedEndDate.getTime()) &&
			parsedStartDate.getTime() <= parsedEndDate.getTime()
		) {
			dateRange = {
				startDate: parsedStartDate,
				endDate: parsedEndDate,
			};
			hasExplicitDateRange = true;
		}
	}

	const onDateRangeChange = (value: DateRangeValue) => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.set(startDateSearchParam, value.startDate.toISOString());
			next.set(endDateSearchParam, value.endDate.toISOString());
			return next;
		});
	};

	const summaryQuery = useQuery({
		...chatCostSummary(user?.id ?? "me", {
			start_date: dateRange.startDate.toISOString(),
			end_date: dateRange.endDate.toISOString(),
		}),
		enabled: Boolean(user?.id),
	});

	return (
		<ScrollArea className="min-h-0 flex-1" viewportClassName="[&>div]:!block">
			<AgentPageHeader mobileBack={{ to: "/agents", label: "Agents" }} />
			<AgentAnalyticsPageView
				summary={summaryQuery.data}
				isLoading={summaryQuery.isLoading}
				error={summaryQuery.error}
				onRetry={() => void summaryQuery.refetch()}
				dateRange={dateRange}
				onDateRangeChange={onDateRangeChange}
				hasExplicitDateRange={hasExplicitDateRange}
			/>
		</ScrollArea>
	);
};

export default AgentAnalyticsPage;
