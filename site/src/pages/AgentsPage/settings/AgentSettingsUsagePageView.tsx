import dayjs from "dayjs";
import { ChevronLeftIcon } from "lucide-react";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { getErrorMessage } from "#/api/errors";
import { chatCostSummary, chatCostUsers } from "#/api/queries/chats";
import { user } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import { PaginationAmount } from "#/components/PaginationWidget/PaginationAmount";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import { SearchField } from "#/components/SearchField/SearchField";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { useDebouncedValue } from "#/hooks/debounce";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { formatTokenCount } from "#/utils/analytics";
import { formatCostMicros } from "#/utils/currency";
import { AdminBadge } from "../components/AdminBadge";
import { ChatCostSummaryView } from "../components/ChatCostSummaryView";
import { SectionHeader } from "../components/SectionHeader";

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

const formatUsageDateRange = (
	value: DateRangeValue,
	options?: {
		endDateIsExclusive?: boolean;
	},
) => {
	// Custom ranges keep the raw API end boundary, which can be midnight on
	// the following day for full-day selections. Show the inclusive day in
	// the drill-in label without changing the query params.
	const displayEndDate =
		options?.endDateIsExclusive &&
		dayjs(value.endDate).isSame(dayjs(value.endDate).startOf("day"))
			? dayjs(value.endDate).subtract(1, "day")
			: dayjs(value.endDate);

	return `${dayjs(value.startDate).format("MMM D")} – ${displayEndDate.format(
		"MMM D, YYYY",
	)}`;
};

const UserRow: FC<{
	user: TypesGen.ChatCostUserRollup;
	onSelect: (user: TypesGen.ChatCostUserRollup) => void;
}> = ({ user, onSelect }) => {
	const clickableRowProps = useClickableTableRow({
		onClick: () => onSelect(user),
	});

	return (
		<TableRow
			{...clickableRowProps}
			aria-label={`View details for ${user.name || user.username}`}
		>
			<TableCell className="min-w-[220px] px-4 py-3">
				<AvatarData
					title={user.name || user.username}
					subtitle={`@${user.username}`}
					src={user.avatar_url}
					imgFallbackText={user.username}
				/>
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatCostMicros(user.total_cost_micros)}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{user.message_count.toLocaleString()}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{user.chat_count.toLocaleString()}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatTokenCount(user.total_input_tokens)}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatTokenCount(user.total_output_tokens)}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatTokenCount(user.total_cache_read_tokens)}
			</TableCell>
			<TableCell className="px-4 py-3 text-right">
				{formatTokenCount(user.total_cache_creation_tokens)}
			</TableCell>
		</TableRow>
	);
};

interface AgentSettingsUsagePageViewProps {
	now?: dayjs.Dayjs;
}

export const AgentSettingsUsagePageView: FC<
	AgentSettingsUsagePageViewProps
> = ({ now }) => {
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
	const { endDate } = dateRange;
	const isExclusiveMidnightEnd =
		hasExplicitDateRange &&
		endDate.getHours() === 0 &&
		endDate.getMinutes() === 0 &&
		endDate.getSeconds() === 0 &&
		endDate.getMilliseconds() === 0;
	const displayDateRange = isExclusiveMidnightEnd
		? {
				startDate: dateRange.startDate,
				endDate: new Date(endDate.getTime() - 1),
			}
		: dateRange;
	const dateRangeLabel = formatUsageDateRange(dateRange, {
		endDateIsExclusive: hasExplicitDateRange,
	});
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
	const selectedUser = selectedUserQuery.data ?? null;

	const summaryQuery = useQuery({
		...chatCostSummary(selectedUserId ?? "me", dateRangeParams),
		enabled: selectedUserId !== null,
	});
	const totalCount = usersQuery.data?.count ?? 0;
	const hasPreviousPage = page > 1;
	const hasNextPage = offset + pageSize < totalCount;

	const header = (
		<SectionHeader
			label="Usage"
			description={
				selectedUserId
					? "Review deployment chat usage for a specific user."
					: "Review deployment chat usage and drill into individual users."
			}
			badge={<AdminBadge />}
			action={
				<DateRangePicker
					value={displayDateRange}
					onChange={onDateRangeChange}
					now={now?.toDate()}
				/>
			}
		/>
	);

	if (selectedUserId) {
		const clearUser = () => {
			setSearchParams((prev) => {
				const next = new URLSearchParams(prev);
				next.delete("user");
				return next;
			});
		};

		const backButton = (
			<button
				type="button"
				onClick={clearUser}
				className="mb-4 inline-flex cursor-pointer items-center gap-0.5 bg-transparent border-0 p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
			>
				{" "}
				<ChevronLeftIcon className="h-4 w-4" />
				Back
			</button>
		);

		if (selectedUserQuery.isLoading) {
			return (
				<div className="space-y-6">
					<div>
						{backButton}
						{header}
					</div>
					<div className="flex min-h-[240px] items-center justify-center">
						<Spinner size="lg" loading className="text-content-secondary" />
					</div>
				</div>
			);
		}

		if (selectedUserQuery.isError || !selectedUser) {
			return (
				<div className="space-y-6">
					<div>
						{backButton}
						{header}
					</div>
					<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
						<p className="m-0 text-sm text-content-secondary">
							{getErrorMessage(
								selectedUserQuery.error,
								"Failed to load user profile.",
							)}
						</p>
						<Button
							variant="outline"
							size="sm"
							type="button"
							onClick={() => void selectedUserQuery.refetch()}
						>
							Retry
						</Button>
					</div>
				</div>
			);
		}

		return (
			<div className="space-y-6">
				<div>
					{backButton}
					{header}
				</div>
				<div className="flex flex-wrap items-center gap-3 rounded-lg border border-border-default bg-surface-secondary px-4 py-3">
					<AvatarData
						title={selectedUser.name || selectedUser.username}
						subtitle={`@${selectedUser.username}`}
						src={selectedUser.avatar_url}
						imgFallbackText={selectedUser.username}
					/>
					<div className="min-w-0 text-xs text-content-secondary">
						<div>User ID: {selectedUser.id}</div>
						<div>{dateRangeLabel}</div>
					</div>
				</div>
				<ChatCostSummaryView
					summary={summaryQuery.data}
					isLoading={summaryQuery.isLoading}
					error={summaryQuery.error}
					onRetry={() => void summaryQuery.refetch()}
					loadingLabel="Loading usage details"
					emptyMessage="No usage data for this user in the selected period."
				/>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{header}
			<div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
				<div className="w-full md:max-w-sm">
					<SearchField
						value={searchFilter}
						onChange={(value) => {
							setSearchFilter(value);
							setPage(1);
						}}
						placeholder="Search by name or username"
						aria-label="Search usage by name or username"
					/>
				</div>
				{usersQuery.data && (
					<PaginationAmount
						limit={pageSize}
						totalRecords={usersQuery.data.count}
						currentOffsetStart={usersQuery.data.count === 0 ? 0 : offset + 1}
						paginationUnitLabel="users"
					/>
				)}
			</div>
			{usersQuery.isLoading && (
				<div
					role="status"
					aria-label="Loading usage"
					className="flex min-h-[240px] items-center justify-center"
				>
					<Spinner size="lg" loading className="text-content-secondary" />
				</div>
			)}

			{usersQuery.error != null && (
				<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
					<p className="m-0 text-sm text-content-secondary">
						{getErrorMessage(usersQuery.error, "Failed to load usage data.")}
					</p>
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={() => void usersQuery.refetch()}
					>
						Retry
					</Button>
				</div>
			)}

			{usersQuery.data && (
				<div className="relative">
					{usersQuery.isFetching && !usersQuery.isLoading && (
						<div
							role="status"
							aria-label="Refreshing usage"
							className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
						>
							<Spinner size="lg" loading className="text-content-secondary" />
						</div>
					)}
					{usersQuery.data.users.length === 0 ? (
						<p className="py-12 text-center text-content-secondary">
							No usage data for this period.
						</p>
					) : (
						<>
							<div className="overflow-hidden rounded-lg border border-border-default">
								<Table>
									<TableHeader>
										<TableRow className="text-left text-xs uppercase tracking-wide text-content-secondary">
											<TableHead className="px-4 py-3">User</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Total Cost
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Messages
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Chats
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Input Tokens
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Output Tokens
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Cache Read
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Cache Write
											</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{usersQuery.data.users.map((user) => (
											<UserRow
												key={user.user_id}
												user={user}
												onSelect={(u) => {
													setSearchParams((prev) => {
														const next = new URLSearchParams(prev);
														next.set("user", u.user_id);
														return next;
													});
												}}
											/>
										))}
									</TableBody>
								</Table>
							</div>
							<PaginationWidgetBase
								totalRecords={usersQuery.data.count}
								currentPage={page}
								pageSize={pageSize}
								onPageChange={setPage}
								hasPreviousPage={hasPreviousPage}
								hasNextPage={hasNextPage}
							/>
						</>
					)}
				</div>
			)}
		</div>
	);
};
