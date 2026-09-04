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
import { SpendDrillInView } from "./components/SpendDrillInView";
import { SpendUsersTable } from "./components/SpendUsersTable";
import { formatUsageDateRange, toInclusiveDateRange } from "./utils/dateRange";

export type SpendUsersQuery = PaginationResult & {
	data: TypesGen.AIGatewaySpendUsersResponse | undefined;
	isLoading: boolean;
	isFetching: boolean;
	error: unknown;
	refetch: () => unknown;
};

interface SpendPageViewProps {
	isEntitled: boolean;
	isEnabled: boolean;
	dateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
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
	dateRange,
	onDateRangeChange,
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

	// Both the default range and URL ranges carry the exclusive API end
	// boundary that DateRangePicker emits.
	const displayDateRange = toInclusiveDateRange(dateRange, true);
	const dateRangeLabel = formatUsageDateRange(dateRange, {
		endDateIsExclusive: true,
	});

	if (drillInUserId) {
		return (
			<SpendDrillInView
				selectedUser={drillInUser}
				isLoading={isDrillInUserLoading}
				error={drillInUserError}
				onRetry={onDrillInUserRetry}
				onBack={onClearSelectedUser}
				displayDateRange={displayDateRange}
				queryDateRange={dateRange}
				onDateRangeChange={onDateRangeChange}
				dateRangeLabel={dateRangeLabel}
				summaryData={summaryData}
				isSummaryLoading={isSummaryLoading}
				summaryError={summaryError}
				onSummaryRetry={onSummaryRetry}
			/>
		);
	}

	return (
		<div className="flex max-w-[1100px] flex-col gap-8">
			<SettingsHeader>
				<SettingsHeaderTitle>AI spend</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Monitor AI Gateway spend across your deployment.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<SpendUsersTable
				displayDateRange={displayDateRange}
				onDateRangeChange={onDateRangeChange}
				searchFilter={searchFilter}
				onSearchFilterChange={onSearchFilterChange}
				usersQuery={usersQuery}
			/>
		</div>
	);
};
