import { chatCostSummary } from "api/queries/chats";
import { useAuthContext } from "contexts/auth/AuthProvider";
import dayjs from "dayjs";
import { BarChart3Icon } from "lucide-react";
import { type FC, useMemo } from "react";
import { useQuery } from "react-query";
import { ChatCostSummaryView } from "./ChatCostSummaryView";
import { SectionHeader } from "./SectionHeader";

const createDateRange = (now?: dayjs.Dayjs) => {
	const end = now ?? dayjs();
	const start = end.subtract(30, "day");
	return {
		startDate: start.toISOString(),
		endDate: end.toISOString(),
		rangeLabel: `${start.format("MMM D")} – ${end.format("MMM D, YYYY")}`,
	};
};

interface AnalyticsPageContentProps {
	/** Override the current time for date range calculation.
	 * Used for deterministic Storybook snapshots.
	 */
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
		<div className="flex min-h-0 flex-1 flex-col overflow-y-auto p-4 pt-8 [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
			<div className="mx-auto w-full max-w-3xl">
				<SectionHeader
					label="Analytics"
					description="Review your personal chat usage and cost breakdowns."
					action={
						<div className="flex items-center gap-2 text-xs text-content-secondary">
							<BarChart3Icon className="h-4 w-4" />
							<span>{dateRange.rangeLabel}</span>
						</div>
					}
				/>

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
