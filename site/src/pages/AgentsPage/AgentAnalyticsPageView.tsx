import type { FC } from "react";
import type { ChatCostSummary } from "#/api/typesGenerated";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import { ChatCostSummaryView } from "./components/ChatCostSummaryView";
import { SectionHeader } from "./components/SectionHeader";
import { toInclusiveDateRange } from "./utils/dateRange";

interface AgentAnalyticsPageViewProps {
	summary: ChatCostSummary | undefined;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
	dateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	hasExplicitDateRange: boolean;
}

export const AgentAnalyticsPageView: FC<AgentAnalyticsPageViewProps> = ({
	summary,
	isLoading,
	error,
	onRetry,
	dateRange,
	onDateRangeChange,
	hasExplicitDateRange,
}) => {
	const displayDateRange = toInclusiveDateRange(
		dateRange,
		hasExplicitDateRange,
	);

	return (
		<div className="flex flex-col p-4 pt-8">
			<div className="mx-auto w-full max-w-3xl">
				<SectionHeader
					label="Analytics"
					description="Review your personal Coder Agents usage and cost breakdowns."
					action={
						<DateRangePicker
							value={displayDateRange}
							onChange={onDateRangeChange}
						/>
					}
				/>

				<ChatCostSummaryView
					summary={summary}
					isLoading={isLoading}
					error={error}
					onRetry={onRetry}
					loadingLabel="Loading analytics"
					emptyMessage="No usage data for you in this period."
				/>
			</div>
		</div>
	);
};
