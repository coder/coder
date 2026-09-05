import dayjs from "dayjs";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { useLocation, useNavigate, useSearchParams } from "react-router";
import {
	aiGatewaySpendUserSummary,
	paginatedAIGatewaySpendUsers,
} from "#/api/queries/aiBridge";
import { user } from "#/api/queries/users";
import {
	type DateRangeValue,
	toBoundary,
} from "#/components/DateRangePicker/DateRangePicker";
import { useDebouncedValue } from "#/hooks/debounce";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { getAIBridgePermissions } from "#/pages/AIBridgePage/getAIBridgePermissions";
import { pageTitle } from "#/utils/page";
import {
	spendListSearchFromState,
	userSearchParam,
} from "./components/SpendUsersTable";
import { SpendPageView } from "./SpendPageView";

const startDateSearchParam = "startDate";
const endDateSearchParam = "endDate";
const DEFAULT_DATE_RANGE_DAYS = 30;
const SEARCH_DEBOUNCE_MS = 300;
const SPEND_USERS_PAGE_SIZE = 10;

// Same calendar-day boundaries as the picker's "Last 30 days" preset, so the
// default range is what that preset would select.
const getDefaultDateRange = (now?: dayjs.Dayjs): DateRangeValue => {
	const current = (now ?? dayjs()).toDate();
	return toBoundary(
		dayjs(current)
			.subtract(DEFAULT_DATE_RANGE_DAYS - 1, "day")
			.toDate(),
		current,
		current,
	);
};

interface SpendPageProps {
	now?: dayjs.Dayjs;
}

const SpendPage: FC<SpendPageProps> = ({ now }) => {
	const { permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const { isEntitled, isEnabled, hasPermission } = getAIBridgePermissions(
		entitlements,
		permissions,
	);
	const canViewSpend = isEntitled && isEnabled && hasPermission;

	const [searchParams, setSearchParams] = useSearchParams();
	const location = useLocation();
	const navigate = useNavigate();

	const searchFilter = searchParams.get("search") ?? "";
	const debouncedSearch = useDebouncedValue(searchFilter, SEARCH_DEBOUNCE_MS);

	const setSearchFilter = (value: string) => {
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				if (value) {
					next.set("search", value);
				} else {
					next.delete("search");
				}
				next.delete("page");
				return next;
			},
			{ replace: true },
		);
	};

	const startDateParam = searchParams.get(startDateSearchParam)?.trim() ?? "";
	const endDateParam = searchParams.get(endDateSearchParam)?.trim() ?? "";

	// The default range is fixed per mount so query keys stay stable while
	// the page is open.
	const [defaultDateRange] = useState(() => getDefaultDateRange(now));
	let dateRange = defaultDateRange;

	if (startDateParam && endDateParam) {
		const parsedStartDate = new Date(startDateParam);
		const parsedEndDate = new Date(endDateParam);

		if (
			!Number.isNaN(parsedStartDate.getTime()) &&
			!Number.isNaN(parsedEndDate.getTime()) &&
			parsedStartDate.getTime() < parsedEndDate.getTime()
		) {
			dateRange = {
				startDate: parsedStartDate,
				endDate: parsedEndDate,
			};
		}
	}

	const dateRangeParams = {
		start_date: dateRange.startDate.toISOString(),
		end_date: dateRange.endDate.toISOString(),
	};

	// DateRangePicker already emits exclusive API boundaries (midnight after
	// the picked day, or the next hour when the picked day is today).
	const onDateRangeChange = (value: DateRangeValue) => {
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				next.set(startDateSearchParam, value.startDate.toISOString());
				next.set(endDateSearchParam, value.endDate.toISOString());
				next.delete("page");
				return next;
			},
			// Keep the drill-in's origin state across range changes.
			{ replace: true, state: location.state },
		);
	};

	const selectedUserId = searchParams.get(userSearchParam) || null;

	// The deployment-wide list is hidden behind a drill-in, so do not keep
	// fetching it there.
	const usersQuery = usePaginatedQuery({
		...paginatedAIGatewaySpendUsers({
			...dateRangeParams,
			search: debouncedSearch,
		}),
		recordsPerPage: SPEND_USERS_PAGE_SIZE,
		preventScrollReset: true,
		enabled: canViewSpend && selectedUserId === null,
	});

	const selectedUserQuery = useQuery({
		...user(selectedUserId ?? ""),
		enabled: canViewSpend && selectedUserId !== null,
	});

	const summaryQuery = useQuery({
		...aiGatewaySpendUserSummary(selectedUserId ?? "", dateRangeParams),
		enabled: canViewSpend && selectedUserId !== null,
	});

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("AI Spend")}</title>
			<SpendPageView
				isEntitled={isEntitled}
				isEnabled={isEnabled}
				dateRange={dateRange}
				onDateRangeChange={onDateRangeChange}
				searchFilter={searchFilter}
				onSearchFilterChange={setSearchFilter}
				usersQuery={usersQuery}
				drillInUserId={selectedUserId}
				drillInUser={selectedUserQuery.data ?? null}
				isDrillInUserLoading={selectedUserQuery.isLoading}
				drillInUserError={selectedUserQuery.error}
				onDrillInUserRetry={() => void selectedUserQuery.refetch()}
				onClearSelectedUser={() => {
					const next = new URLSearchParams(searchParams);
					next.delete(userSearchParam);
					// Pop the drill-in when the entry beneath it is the list Back
					// would show. Direct links and filters changed inside the
					// drill-in replace the entry instead so the list reflects them.
					if (spendListSearchFromState(location.state) === next.toString()) {
						navigate(-1);
						return;
					}
					setSearchParams(next, { replace: true });
				}}
				summaryData={summaryQuery.data}
				isSummaryLoading={summaryQuery.isLoading}
				summaryError={summaryQuery.error}
				onSummaryRetry={() => void summaryQuery.refetch()}
			/>
		</RequirePermission>
	);
};

export default SpendPage;
