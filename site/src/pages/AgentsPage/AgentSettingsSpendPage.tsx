import dayjs from "dayjs";
import type { FC } from "react";
import {
	keepPreviousData,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useSearchParams } from "react-router";
import {
	chatCostSummary,
	chatCostUsers,
	chatUsageLimitConfig,
	deleteChatUsageLimitGroupOverride,
	deleteChatUsageLimitOverride,
	updateChatUsageLimitConfig,
	upsertChatUsageLimitGroupOverride,
	upsertChatUsageLimitOverride,
} from "#/api/queries/chats";
import { groups } from "#/api/queries/groups";
import { user } from "#/api/queries/users";
import type { ChatCostUserRollup } from "#/api/typesGenerated";
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { useDebouncedValue } from "#/hooks/debounce";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { AgentSettingsSpendPageView } from "./AgentSettingsSpendPageView";

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

interface AgentSettingsSpendPageProps {
	/** Override the current time for date range calculation. Used for
	 *  deterministic Storybook snapshots. */
	now?: dayjs.Dayjs;
}

const AgentSettingsSpendPage: FC<AgentSettingsSpendPageProps> = ({ now }) => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();

	// --------------- Limits queries & mutations ---------------

	const configQuery = useQuery(chatUsageLimitConfig());
	const groupsQuery = useQuery(groups());

	const updateConfigMutation = useMutation(
		updateChatUsageLimitConfig(queryClient),
	);
	const upsertOverrideMutation = useMutation(
		upsertChatUsageLimitOverride(queryClient),
	);
	const deleteOverrideMutation = useMutation(
		deleteChatUsageLimitOverride(queryClient),
	);
	const upsertGroupOverrideMutation = useMutation(
		upsertChatUsageLimitGroupOverride(queryClient),
	);
	const deleteGroupOverrideMutation = useMutation(
		deleteChatUsageLimitGroupOverride(queryClient),
	);

	// --------------- Usage state & queries ---------------

	const [searchParams, setSearchParams] = useSearchParams();

	// All ephemeral UI state lives in URL search params so the page
	// is deep-linkable and survives navigation.
	const searchFilter = searchParams.get("search") ?? "";
	const debouncedSearch = useDebouncedValue(searchFilter, 300);
	const page = Number(searchParams.get("page") ?? "1") || 1;

	const setSearchFilter = (value: string) => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			if (value) {
				next.set("search", value);
			} else {
				next.delete("search");
			}
			// Reset to page 1 when the search changes.
			next.delete("page");
			return next;
		});
	};

	const setPage = (newPage: number) => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			if (newPage > 1) {
				next.set("page", String(newPage));
			} else {
				next.delete("page");
			}
			return next;
		});
	};

	const startDateParam =
		searchParams.get(usageStartDateSearchParam)?.trim() ?? "";
	const endDateParam = searchParams.get(usageEndDateSearchParam)?.trim() ?? "";

	// Capture the default once so it doesn't shift on re-render.
	const defaultDateRange = getDefaultUsageDateRange(now);
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
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.set(usageStartDateSearchParam, value.startDate.toISOString());
			next.set(usageEndDateSearchParam, value.endDate.toISOString());
			// Reset pagination when date range changes.
			next.delete("page");
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
			<AgentSettingsSpendPageView
				// Limits config
				configData={configQuery.data}
				isLoadingConfig={configQuery.isLoading}
				configError={configQuery.isError ? configQuery.error : null}
				refetchConfig={() => void configQuery.refetch()}
				groupsData={groupsQuery.data}
				isLoadingGroups={groupsQuery.isLoading}
				groupsError={groupsQuery.isError ? groupsQuery.error : null}
				onUpdateConfig={updateConfigMutation.mutate}
				isUpdatingConfig={updateConfigMutation.isPending}
				updateConfigError={
					updateConfigMutation.isError ? updateConfigMutation.error : null
				}
				isUpdateConfigSuccess={updateConfigMutation.isSuccess}
				resetUpdateConfig={updateConfigMutation.reset}
				onUpsertOverride={({ userID, req, onSuccess }) =>
					upsertOverrideMutation.mutate({ userID, req }, { onSuccess })
				}
				isUpsertingOverride={upsertOverrideMutation.isPending}
				upsertOverrideError={
					upsertOverrideMutation.isError ? upsertOverrideMutation.error : null
				}
				onDeleteOverride={deleteOverrideMutation.mutate}
				isDeletingOverride={deleteOverrideMutation.isPending}
				deleteOverrideError={
					deleteOverrideMutation.isError ? deleteOverrideMutation.error : null
				}
				onUpsertGroupOverride={({ groupID, req, onSuccess }) =>
					upsertGroupOverrideMutation.mutate({ groupID, req }, { onSuccess })
				}
				isUpsertingGroupOverride={upsertGroupOverrideMutation.isPending}
				upsertGroupOverrideError={
					upsertGroupOverrideMutation.isError
						? upsertGroupOverrideMutation.error
						: null
				}
				onDeleteGroupOverride={deleteGroupOverrideMutation.mutate}
				isDeletingGroupOverride={deleteGroupOverrideMutation.isPending}
				deleteGroupOverrideError={
					deleteGroupOverrideMutation.isError
						? deleteGroupOverrideMutation.error
						: null
				}
				// Usage data
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
				onSelectUser={(u: ChatCostUserRollup) => {
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

export default AgentSettingsSpendPage;
