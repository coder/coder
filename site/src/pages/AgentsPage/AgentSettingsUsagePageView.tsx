import dayjs from "dayjs";
import { ChevronLeftIcon } from "lucide-react";
import type { FC } from "react";
import { getErrorMessage } from "#/api/errors";
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
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { formatTokenCount } from "#/utils/analytics";
import { formatCostMicros } from "#/utils/currency";
import { AdminBadge } from "./components/AdminBadge";
import { ChatCostSummaryView } from "./components/ChatCostSummaryView";
import { SectionHeader } from "./components/SectionHeader";

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
	// Raw date range (parsed by Page from URL params)
	dateRange: DateRangeValue;
	hasExplicitDateRange: boolean;
	onDateRangeChange: (value: DateRangeValue) => void;

	// Search & pagination (state owned by Page, needed for queries)
	searchFilter: string;
	onSearchFilterChange: (value: string) => void;
	page: number;
	onPageChange: (page: number) => void;
	pageSize: number;
	offset: number;

	// User list query
	usersData: TypesGen.ChatCostUsersResponse | undefined;
	isUsersLoading: boolean;
	isUsersFetching: boolean;
	usersError: unknown;
	onUsersRetry: () => void;

	// Selected user drill-in
	selectedUserId: string | null;
	selectedUser: TypesGen.User | null;
	isSelectedUserLoading: boolean;
	isSelectedUserError: boolean;
	selectedUserError: unknown;
	onSelectedUserRetry: () => void;
	onClearSelectedUser: () => void;
	onSelectUser: (user: TypesGen.ChatCostUserRollup) => void;

	// Cost summary for selected user
	summaryData: TypesGen.ChatCostSummary | undefined;
	isSummaryLoading: boolean;
	summaryError: unknown;
	onSummaryRetry: () => void;
}

export const AgentSettingsUsagePageView: FC<
	AgentSettingsUsagePageViewProps
> = ({
	dateRange,
	hasExplicitDateRange,
	onDateRangeChange,
	searchFilter,
	onSearchFilterChange,
	page,
	onPageChange,
	pageSize,
	offset,
	usersData,
	isUsersLoading,
	isUsersFetching,
	usersError,
	onUsersRetry,
	selectedUserId,
	selectedUser,
	isSelectedUserLoading,
	isSelectedUserError,
	selectedUserError,
	onSelectedUserRetry,
	onClearSelectedUser,
	onSelectUser,
	summaryData,
	isSummaryLoading,
	summaryError,
	onSummaryRetry,
}) => {
	// ── Derived display state ──
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
	const totalCount = usersData?.count ?? 0;
	const hasPreviousPage = page > 1;
	const hasNextPage = offset + pageSize < totalCount;

	const header = (
		<SectionHeader
			label="Usage"
			description={
				selectedUserId
					? "Review deployment Coder Agents usage for a specific user."
					: "Review deployment Coder Agents usage and drill into individual users."
			}
			badge={<AdminBadge />}
			action={
				<DateRangePicker
					value={displayDateRange}
					onChange={onDateRangeChange}
				/>
			}
		/>
	);

	if (selectedUserId) {
		const backButton = (
			<button
				type="button"
				onClick={onClearSelectedUser}
				className="mb-4 inline-flex cursor-pointer items-center gap-0.5 bg-transparent border-0 p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
			>
				{" "}
				<ChevronLeftIcon className="h-4 w-4" />
				Back
			</button>
		);

		if (isSelectedUserLoading) {
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

		if (isSelectedUserError || !selectedUser) {
			return (
				<div className="space-y-6">
					<div>
						{backButton}
						{header}
					</div>
					<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
						<p className="m-0 text-sm text-content-secondary">
							{getErrorMessage(
								selectedUserError,
								"Failed to load user profile.",
							)}
						</p>{" "}
						<Button
							variant="outline"
							size="sm"
							type="button"
							onClick={onSelectedUserRetry}
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
					summary={summaryData}
					isLoading={isSummaryLoading}
					error={summaryError}
					onRetry={onSummaryRetry}
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
							onSearchFilterChange(value);
							onPageChange(1);
						}}
						placeholder="Search by name or username"
						aria-label="Search usage by name or username"
					/>
				</div>
				{usersData && (
					<PaginationAmount
						limit={pageSize}
						totalRecords={usersData.count}
						currentOffsetStart={usersData.count === 0 ? 0 : offset + 1}
						paginationUnitLabel="users"
					/>
				)}
			</div>
			{isUsersLoading && (
				<div
					role="status"
					aria-label="Loading usage"
					className="flex min-h-[240px] items-center justify-center"
				>
					<Spinner size="lg" loading className="text-content-secondary" />
				</div>
			)}

			{usersError != null && (
				<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
					<p className="m-0 text-sm text-content-secondary">
						{getErrorMessage(usersError, "Failed to load usage data.")}
					</p>{" "}
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={onUsersRetry}
					>
						Retry
					</Button>
				</div>
			)}

			{usersData && (
				<div className="relative">
					{isUsersFetching && !isUsersLoading && (
						<div
							role="status"
							aria-label="Refreshing usage"
							className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
						>
							<Spinner size="lg" loading className="text-content-secondary" />
						</div>
					)}
					{usersData.users.length === 0 ? (
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
										{usersData.users.map((user) => (
											<UserRow
												key={user.user_id}
												user={user}
												onSelect={onSelectUser}
											/>
										))}
									</TableBody>
								</Table>
							</div>
							<PaginationWidgetBase
								totalRecords={usersData.count}
								currentPage={page}
								pageSize={pageSize}
								onPageChange={onPageChange}
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
