import { chatCostSummary } from "api/queries/chats";
import { useAuthContext } from "contexts/auth/AuthProvider";
import dayjs from "dayjs";
import { type FC, useMemo } from "react";
import { useQuery } from "react-query";
import { ChatCostSummaryView } from "./ChatCostSummaryView";

const createDateRange = (now?: dayjs.Dayjs) => {
	const end = now ?? dayjs();
	const start = end.subtract(30, "day");
	return {
		startDate: start.toISOString(),
		endDate: end.toISOString(),
	};
};

interface AnalyticsPageContentProps {
	now?: dayjs.Dayjs;
}

export const AnalyticsPageContent: FC<AnalyticsPageContentProps> = ({
	now,
}) => {
	const { user } = useAuthContext();
	const dateRange = useMemo(() => createDateRange(now), [now]);

	const summaryQuery = useQuery({
		...chatCostSummary(user?.id ?? "me", {
			start_date: dateRange.startDate,
			end_date: dateRange.endDate,
		}),
		enabled: Boolean(user?.id),
	});

	return (
		<div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-4 pt-10 pb-16 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
			<div className="mx-auto w-full max-w-4xl">
				<div className="mb-8">
					<h1 className="text-2xl font-semibold text-content-primary">
						Analytics
					</h1>
					<p className="m-0 text-sm text-content-secondary">
						Personal usage over the last 30 days
					</p>
				</div>
				<ChatCostSummaryView
					summary={summaryQuery.data}
					isLoading={summaryQuery.isLoading}
					error={summaryQuery.error}
					onRetry={() => void summaryQuery.refetch()}
					loadingLabel="Loading analytics"
					emptyMessage="No usage data for you in this period."
				/>
			</div>
		</div>
	);
};
