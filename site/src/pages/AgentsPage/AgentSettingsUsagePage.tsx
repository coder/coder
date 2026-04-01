import dayjs from "dayjs";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { chatCostSummary, chatCostUsers } from "#/api/queries/chats";
import { user } from "#/api/queries/users";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { useDebouncedValue } from "#/hooks/debounce";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsUsagePageView } from "./AgentSettingsUsagePageView";

const pageSize = 10;

const usageStartDateSearchParam = "startDate";
const usageEndDateSearchParam = "endDate";

const getDefaultUsageDateRange = (now?: dayjs.Dayjs): DateRangeValue => {
	const end = now ?? dayjs();
	return {
		startDate: end.subtract(30, "day").toDate(),
		endDate: end.toDate(),
	};
};

interface AgentSettingsUsagePageProps {
	/** Override the current time for date range calculation. Used for
	 *  deterministic Storybook snapshots. */
	now?: dayjs.Dayjs;
}

const AgentSettingsUsagePage: FC<AgentSettingsUsagePageProps> = ({ now }) => {
	const { permissions } = useAuthenticated();

	const [searchParams, setSearchParams] = useSearchParams();
	const [searchFilter, setSearchFilter] = useState("");
	const debouncedSearch = useDebouncedValue(searchFilter, 300);
	const [page, setPage] = useState(1);
	const startDateParam =
		searchParams.get(usageStartDateSearchParam)?.trim() ?? "";
	const endDateParam = searchParams.get(usageEndDateSearchParam)?.trim() ?? "";
	const [defaultDateRange] = useState(() => getDefaultUsageDateRange(now));
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

	const dateRangeParams = {
		start_date: dateRange.startDate.toISOString(),
		end_date: dateRange.endDate.toISOString(),
	};
	const offset = (page - 1) * pageSize;

	const onDateRangeChange = (value: DateRangeValue) => {
		// Reset pagination but preserve user selection and other params.
		setPage(1);
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.set(usageStartDateSearchParam, value.startDate.toISOString());
			next.set(usageEndDateSearchParam, value.endDate.toISOString());
			return next;
		});
	};

	const usersQuery = useQuery({
		...chatCostUsers({
			...dateRangeParams,
			username: debouncedSearch || undefined,
			limit: pageSize,
			offset,
		}),
		placeholderData: keepPreviousData,
	});

	const selectedUserId = searchParams.get("user");
	const selectedUserQuery = useQuery({
		...user(selectedUserId ?? ""),
		enabled: selectedUserId !== null,
	});

	const summaryQuery = useQuery({
		...chatCostSummary(selectedUserId ?? "me", dateRangeParams),
		enabled: selectedUserId !== null,
	});

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<AgentSettingsUsagePageView
				dateRange={dateRange}
				hasExplicitDateRange={hasExplicitDateRange}
				onDateRangeChange={onDateRangeChange}
				searchFilter={searchFilter}
				onSearchFilterChange={setSearchFilter}
				page={page}
				onPageChange={setPage}
				pageSize={pageSize}
				offset={offset}
				usersData={usersQuery.data}
				isUsersLoading={usersQuery.isLoading}
				isUsersFetching={usersQuery.isFetching}
				usersError={usersQuery.error}
				onUsersRetry={() => void usersQuery.refetch()}
				selectedUserId={selectedUserId}
				selectedUser={selectedUserQuery.data ?? null}
				isSelectedUserLoading={selectedUserQuery.isLoading}
				isSelectedUserError={selectedUserQuery.isError}
				selectedUserError={selectedUserQuery.error}
				onSelectedUserRetry={() => void selectedUserQuery.refetch()}
				onClearSelectedUser={() => {
					setSearchParams((prev) => {
						const next = new URLSearchParams(prev);
						next.delete("user");
						return next;
					});
				}}
				onSelectUser={(u) => {
					setSearchParams((prev) => {
						const next = new URLSearchParams(prev);
						next.set("user", u.user_id);
						return next;
					});
				}}
				summaryData={summaryQuery.data}
				isSummaryLoading={summaryQuery.isLoading}
				summaryError={summaryQuery.error}
				onSummaryRetry={() => void summaryQuery.refetch()}
			/>
		</RequirePermission>
	);
};

export default AgentSettingsUsagePage;
