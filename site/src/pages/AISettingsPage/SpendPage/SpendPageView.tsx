import dayjs from "dayjs";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { PremiumPaywallAIGovernance } from "#/modules/paywall/PremiumPaywallAIGovernance";
import { AIBridgeSetupAlert } from "#/pages/AIBridgePage/AIBridgeSetupAlert";
import { RetentionNotice } from "./components/RetentionNotice";
import { SpendDrillInView } from "./components/SpendDrillInView";
import {
	type SpendDimensions,
	type SpendFilterMenus,
	SpendFilters,
} from "./components/SpendFilters";
import { SpendSectionHeader } from "./components/SpendSectionHeader";
import { SpendSummaryView } from "./components/SpendSummaryView";
import { SpendUsersTable } from "./components/SpendUsersTable";

export type SpendUsersQuery = PaginationResult & {
	data: TypesGen.AIGatewaySpendUsersResponse | undefined;
	isLoading: boolean;
	isFetching: boolean;
	error: unknown;
	refetch: () => unknown;
};

// DateRangePicker emits an exclusive end boundary at midnight of the following
// day but labels its button with the raw end date, so the displayed range is
// pulled back by 1 ms to land on the day the user actually selected. A sub-day
// boundary (today, rounded up to the next hour) already sits on the right day.
const toInclusiveDateRange = (range: DateRangeValue): DateRangeValue => {
	const end = dayjs(range.endDate);
	return end.isSame(end.startOf("day"))
		? { startDate: range.startDate, endDate: new Date(end.valueOf() - 1) }
		: range;
};

interface SpendPageViewProps {
	isEntitled: boolean;
	isEnabled: boolean;
	now?: Date;
	dateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	dimensions: SpendDimensions;
	filterMenus: SpendFilterMenus;
	searchFilter: string;
	onSearchFilterChange: (value: string) => void;
	usersQuery: SpendUsersQuery;
	drillInUserId: string | null;
	drillInUser: TypesGen.User | null;
	isDrillInUserLoading: boolean;
	drillInUserError: unknown;
	onDrillInUserRetry: () => void;
	onClearSelectedUser: () => void;
	summaryData: TypesGen.AIGatewaySpendUserSummary | undefined;
	isSummaryLoading: boolean;
	summaryError: unknown;
	onSummaryRetry: () => void;
}

export const SpendPageView: FC<SpendPageViewProps> = ({
	isEntitled,
	isEnabled,
	now,
	dateRange,
	onDateRangeChange,
	dimensions,
	filterMenus,
	searchFilter,
	onSearchFilterChange,
	usersQuery,
	drillInUserId,
	drillInUser,
	isDrillInUserLoading,
	drillInUserError,
	onDrillInUserRetry,
	onClearSelectedUser,
	summaryData,
	isSummaryLoading,
	summaryError,
	onSummaryRetry,
}) => {
	if (!isEntitled) {
		return (
			<PremiumPaywallAIGovernance variant="governance" source="ai_spend" />
		);
	}

	if (!isEnabled) {
		return <AIBridgeSetupAlert />;
	}

	const displayDateRange = toInclusiveDateRange(dateRange);
	const filterProps = {
		menus: filterMenus,
		now,
		dateRange: displayDateRange,
		onDateRangeChange,
	};

	if (drillInUserId) {
		return (
			<SpendDrillInView
				selectedUser={drillInUser}
				isLoading={isDrillInUserLoading}
				error={drillInUserError}
				onRetry={onDrillInUserRetry}
				onBack={onClearSelectedUser}
				filters={<SpendFilters {...filterProps} />}
				dimensions={dimensions}
				queryDateRange={dateRange}
				summaryData={summaryData}
				isSummaryLoading={isSummaryLoading}
				summaryError={summaryError}
				onSummaryRetry={onSummaryRetry}
			/>
		);
	}

	return (
		<div className="flex max-w-[1100px] flex-col gap-8">
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle>AI spend</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Monitor AI Gateway spend across your deployment.
					</SettingsHeaderDescription>
				</SettingsHeader>
				<div className="flex flex-col gap-4">
					<SpendFilters
						{...filterProps}
						search={{ value: searchFilter, onChange: onSearchFilterChange }}
					/>
					<SpendUsersTable
						displayDateRange={displayDateRange}
						searchFilter={searchFilter}
						usersQuery={usersQuery}
					/>
				</div>
			</div>
			<section className="space-y-6">
				<SpendSectionHeader
					title="Deployment spend"
					description="Totals and breakdowns across all users for the selected period and filters, independent of the user search above."
				/>
				{summaryData && (
					<RetentionNotice
						requestedStart={dateRange.startDate}
						applied={summaryData}
					/>
				)}
				<SpendSummaryView
					summary={summaryData}
					isLoading={isSummaryLoading}
					error={summaryError}
					onRetry={onSummaryRetry}
				/>
			</section>
		</div>
	);
};
