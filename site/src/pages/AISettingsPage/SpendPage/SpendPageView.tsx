import dayjs from "dayjs";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { Loader } from "#/components/Loader/Loader";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { PremiumPaywallAIGovernance } from "#/modules/paywall/PremiumPaywallAIGovernance";
import { AIBridgeSetupAlert } from "#/pages/AIBridgePage/AIBridgeSetupAlert";
import { type SpendFilterMenus, SpendFilters } from "./components/SpendFilters";
import { SpendUsersTable } from "./components/SpendUsersTable";

export type SpendUsersQuery = PaginationResult & {
	data: TypesGen.OrganizationAISpendReport | undefined;
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
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization | undefined;
	onOrganizationChange: (organization: TypesGen.Organization) => void;
	requestedOrganizationDenied: boolean;
	isOrganizationsLoading: boolean;
	organizationsError: unknown;
	dateRange: DateRangeValue | undefined;
	minDate: Date | undefined;
	onDateRangeChange: (value: DateRangeValue) => void;
	filterMenus: SpendFilterMenus;
	usersQuery: SpendUsersQuery;
}

export const SpendPageView: FC<SpendPageViewProps> = ({
	isEntitled,
	isEnabled,
	now,
	organizations,
	organization,
	onOrganizationChange,
	requestedOrganizationDenied,
	isOrganizationsLoading,
	organizationsError,
	dateRange,
	minDate,
	onDateRangeChange,
	filterMenus,
	usersQuery,
}) => {
	if (!isEntitled) {
		return (
			<PremiumPaywallAIGovernance variant="governance" source="ai_governance" />
		);
	}

	if (!isEnabled) {
		return <AIBridgeSetupAlert />;
	}

	return (
		<div className="flex max-w-[1100px] flex-col gap-4">
			<SettingsHeader>
				<SettingsHeaderTitle>AI spend</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Monitor AI Gateway spend for one organization at a time. Spend is
					attributed through each user's budget group, so usage outside a group
					is not shown.
				</SettingsHeaderDescription>
			</SettingsHeader>
			{isOrganizationsLoading ? (
				<Loader />
			) : organization === undefined ? (
				organizationsError != null ? (
					<ErrorAlert error={organizationsError} />
				) : (
					<p className="py-12 text-center text-content-secondary">
						You do not have access to any organization's AI spend.
					</p>
				)
			) : (
				<>
					{organizationsError != null && (
						<ErrorAlert error={organizationsError} />
					)}
					<SpendFilters
						organizations={organizations}
						organization={organization}
						onOrganizationChange={onOrganizationChange}
						menus={filterMenus}
						now={now}
						dateRange={dateRange && toInclusiveDateRange(dateRange)}
						minDate={minDate}
						isReportLoading={usersQuery.isLoading}
						onDateRangeChange={onDateRangeChange}
					/>
					{requestedOrganizationDenied && (
						<Alert severity="warning">
							The requested organization is not available. Showing spend for{" "}
							{organization.display_name || organization.name} instead.
						</Alert>
					)}
					<SpendUsersTable usersQuery={usersQuery} />
				</>
			)}
		</div>
	);
};
