import { getErrorMessage } from "api/errors";
import { chatCostSummary, chatCostUsers } from "api/queries/chats";
import { user } from "api/queries/users";
import type {
	ChatCostUserRollup,
	ChatUsageLimitConfigResponse,
} from "api/typesGenerated";
import dayjs, { type Dayjs } from "dayjs";
import { useDebouncedValue } from "hooks/debounce";
import { useClickableTableRow } from "hooks/useClickableTableRow";
import { ChevronLeftIcon } from "lucide-react";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { formatCostMicros } from "utils/currency";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
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
import { AdminBadge } from "../AdminBadge";
import { ChatCostSummaryView } from "../ChatCostSummaryView";
import {
	DateRangePicker,
	type DateRangeValue,
} from "../DateRangePicker/DateRangePicker";
import { SectionHeader } from "../SectionHeader";

// ── Constants ──────────────────────────────────────────────────────

const pageSize = 10;

export const usageStartDateSearchParam = "startDate";
export const usageEndDateSearchParam = "endDate";

// ── Helpers ────────────────────────────────────────────────────────

export const getDefaultUsageDateRange = (now?: Dayjs): DateRangeValue => {
	const end = now ?? dayjs();
	return {
		startDate: end.subtract(30, "day").toDate(),
		endDate: end.toDate(),
	};
};

export const formatUsageDateRange = (
	value: DateRangeValue,
	options?: { endDateIsExclusive?: boolean },
) => {
	const displayEndDate =
		options?.endDateIsExclusive &&
		dayjs(value.endDate).isSame(dayjs(value.endDate).startOf("day"))
			? dayjs(value.endDate).subtract(1, "day")
			: dayjs(value.endDate);

	return `${dayjs(value.startDate).format("MMM D")} – ${displayEndDate.format(
		"MMM D, YYYY",
	)}`;
};

// ── UserRow ────────────────────────────────────────────────────────

const UserRow: FC<{
	user: ChatCostUserRollup;
	effectiveLimit: string;
	onSelect: (user: ChatCostUserRollup) => void;
}> = ({ user, effectiveLimit, onSelect }) => {
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
			<TableCell className="px-4 py-3 text-right">{effectiveLimit}</TableCell>
			<TableCell className="px-4 py-3 text-right">
				<ChevronLeftIcon className="ml-auto h-4 w-4 rotate-180 text-content-secondary" />
			</TableCell>
		</TableRow>
	);
};

// ── UsageUsersSection ──────────────────────────────────────────────

interface UsageUsersSectionProps {
	dateRangeParams: { start_date: string; end_date: string };
	dateRangeLabel: string;
	displayDateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	configData: ChatUsageLimitConfigResponse | undefined;
}

export const UsageUsersSection: FC<UsageUsersSectionProps> = ({
	dateRangeParams,
	dateRangeLabel,
	displayDateRange,
	onDateRangeChange,
	configData,
}) => {
	const [searchParams, setSearchParams] = useSearchParams();

	// ── Search & pagination state ─────────────────────────────────

	const [searchFilter, setSearchFilter] = useState("");
	const debouncedSearch = useDebouncedValue(searchFilter, 300);
	const [page, setPage] = useState(1);

	// Reset page when date range changes.
	const dateRangeKey = `${dateRangeParams.start_date}:${dateRangeParams.end_date}`;
	const [prevDateRangeKey, setPrevDateRangeKey] = useState(dateRangeKey);
	if (prevDateRangeKey !== dateRangeKey) {
		setPrevDateRangeKey(dateRangeKey);
		setPage(1);
	}

	const offset = (page - 1) * pageSize;

	// ── Usage queries ─────────────────────────────────────────────

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

	// ── Effective limit for a usage row ───────────────────────────

	const getEffectiveLimit = (u: ChatCostUserRollup): string => {
		const userOverride = (configData?.overrides ?? []).find(
			(o) => o.user_id === u.user_id,
		);
		if (userOverride) {
			return formatCostMicros(userOverride.spend_limit_micros ?? 0);
		}
		if (configData?.spend_limit_micros != null) {
			return `Default (${formatCostMicros(configData.spend_limit_micros)})`;
		}
		return "Unlimited";
	};

	// ── User drill-in handler ─────────────────────────────────────

	const handleSelectUser = (u: ChatCostUserRollup) => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.set("user", u.user_id);
			return next;
		});
	};

	const clearUser = () => {
		setSearchParams((prev) => {
			const next = new URLSearchParams(prev);
			next.delete("user");
			return next;
		});
	};

	// ── Drill-in: selected user detail view ───────────────────────

	if (selectedUserId) {
		const backButton = (
			<button
				type="button"
				onClick={clearUser}
				className="mb-4 inline-flex cursor-pointer items-center gap-0.5 border-0 bg-transparent p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
			>
				<ChevronLeftIcon className="h-4 w-4" />
				Back
			</button>
		);

		const drillHeader = (
			<SectionHeader
				label="Usage & Limits"
				description="Review deployment chat usage for a specific user."
				badge={<AdminBadge className="ml-auto" />}
				action={
					<DateRangePicker
						value={displayDateRange}
						onChange={onDateRangeChange}
					/>
				}
			/>
		);

		if (selectedUserQuery.isLoading) {
			return (
				<div className="space-y-6">
					<div>
						{backButton}
						{drillHeader}
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
						{drillHeader}
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
					{drillHeader}
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

	// ── Normal view: users table ──────────────────────────────────

	return (
		<div className="space-y-4">
			<div className="flex items-center justify-between gap-4">
				<h3 className="m-0 text-base font-medium text-content-primary">
					Users
				</h3>
				<DateRangePicker
					value={displayDateRange}
					onChange={onDateRangeChange}
				/>
			</div>
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
								<Table aria-label="User usage breakdown">
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
												Spend Limit
											</TableHead>
											<TableHead className="w-10 px-4 py-3" />
										</TableRow>
									</TableHeader>
									<TableBody>
										{usersQuery.data.users.map((u) => (
											<UserRow
												key={u.user_id}
												user={u}
												effectiveLimit={getEffectiveLimit(u)}
												onSelect={handleSelectUser}
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
