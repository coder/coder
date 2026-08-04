import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { Link } from "#/components/Link/Link";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { docs } from "#/utils/docs";
import { SpendDrillInView } from "./components/SpendDrillInView";
import { UsageTab } from "./components/UsageTab/UsageTab";
import { formatUsageDateRange, toInclusiveDateRange } from "./utils/dateRange";

interface SpendPageViewProps {
	dateRange: DateRangeValue;
	endDateIsExclusive: boolean;
	onDateRangeChange: (value: DateRangeValue) => void;
	searchFilter: string;
	onSearchFilterChange: (value: string) => void;
	usersQuery: PaginationResult & {
		data: TypesGen.ChatCostUsersResponse | undefined;
		isLoading: boolean;
		isFetching: boolean;
		error: unknown;
		refetch: () => unknown;
	};
	drillInUserId: string | null;
	drillInUser: TypesGen.User | null;
	isDrillInUserLoading: boolean;
	isDrillInUserError: boolean;
	drillInUserError: unknown;
	onDrillInUserRetry: () => void;
	onClearSelectedUser: () => void;
	onSelectUser: (user: TypesGen.ChatCostUserRollup) => void;
	summaryData: TypesGen.ChatCostSummary | undefined;
	isSummaryLoading: boolean;
	summaryError: unknown;
	onSummaryRetry: () => void;
}

export const SpendPageView: FC<SpendPageViewProps> = ({
	dateRange,
	endDateIsExclusive,
	onDateRangeChange,
	searchFilter,
	onSearchFilterChange,
	usersQuery,
	drillInUserId,
	drillInUser,
	isDrillInUserLoading,
	isDrillInUserError,
	drillInUserError,
	onDrillInUserRetry,
	onClearSelectedUser,
	onSelectUser,
	summaryData,
	isSummaryLoading,
	summaryError,
	onSummaryRetry,
}) => {
	const displayDateRange = toInclusiveDateRange(dateRange, endDateIsExclusive);
	const dateRangeLabel = formatUsageDateRange(dateRange, {
		endDateIsExclusive,
	});

	if (drillInUserId) {
		return (
			<SpendDrillInView
				selectedUser={drillInUser}
				isLoading={isDrillInUserLoading}
				isError={isDrillInUserError}
				error={drillInUserError}
				onRetry={onDrillInUserRetry}
				onBack={onClearSelectedUser}
				displayDateRange={displayDateRange}
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
			<Alert severity="warning" prominent>
				<AlertDescription>
					As of v2.36, AI Governance Cost Control replaces Coder Agents Cost
					Control. The limits on this page are no longer enforced and do not
					carry over. Recreate each limit as an AI Governance budget to restore
					enforcement.{" "}
					<Link
						href={docs(
							"/ai-coder/ai-gateway/cost-controls#migrate-from-coder-agents-cost-control",
						)}
						target="_blank"
						rel="noreferrer"
					>
						Read more here
					</Link>
				</AlertDescription>
			</Alert>

			<SettingsHeader>
				<SettingsHeaderTitle>AI spend usage</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Monitor AI usage across your deployment.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<div className="pt-8">
				<UsageTab
					displayDateRange={displayDateRange}
					onDateRangeChange={onDateRangeChange}
					searchFilter={searchFilter}
					onSearchFilterChange={onSearchFilterChange}
					usersQuery={usersQuery}
					onSelectUser={onSelectUser}
				/>
			</div>
		</div>
	);
};
