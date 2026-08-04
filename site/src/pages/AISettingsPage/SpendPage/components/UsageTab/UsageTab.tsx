import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
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
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { formatTokenCount } from "#/utils/analytics";
import { formatCostMicros } from "#/utils/currency";
import { SpendSectionHeader } from "../SpendSectionHeader";

interface UsageTabProps {
	displayDateRange: DateRangeValue;
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
	onSelectUser: (user: TypesGen.ChatCostUserRollup) => void;
}

export const UsageTab: FC<UsageTabProps> = ({
	displayDateRange,
	onDateRangeChange,
	searchFilter,
	onSearchFilterChange,
	usersQuery,
	onSelectUser,
}) => {
	return (
		<section className="space-y-6">
			<SpendSectionHeader
				title="Usage by user"
				description="Monitor AI usage and spend for users in the selected date range."
				actions={
					<DateRangePicker
						value={displayDateRange}
						onChange={onDateRangeChange}
					/>
				}
			/>
			<div>
				<div className="w-full md:max-w-sm">
					<SearchField
						value={searchFilter}
						onChange={onSearchFilterChange}
						placeholder="Search by name or username"
						aria-label="Search usage by name or username"
					/>
				</div>
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
					<ErrorAlert error={usersQuery.error} />
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
				<div className="relative pt-3">
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
						<PaginationContainer query={usersQuery} paginationUnitLabel="users">
							<div className="overflow-hidden rounded-lg border border-border-default">
								<Table aria-label="User spend details">
									<TableHeader>
										<TableRow>
											<TableHead>User</TableHead>
											<TableHead className="text-right">Cost</TableHead>
											<TableHead className="text-right">Messages</TableHead>
											<TableHead className="text-right">Chats</TableHead>
											<TableHead className="text-right">Input</TableHead>
											<TableHead className="text-right">Output</TableHead>
											<TableHead className="text-right">Cache Read</TableHead>
											<TableHead className="text-right">Cache Write</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{usersQuery.data.users.map((user) => (
											<UserRow
												key={user.user_id}
												user={user}
												onSelect={onSelectUser}
											/>
										))}
									</TableBody>
								</Table>
							</div>
						</PaginationContainer>
					)}
				</div>
			)}
		</section>
	);
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
			className="text-xs"
		>
			<TableCell className="max-w-[200px] px-3 py-2">
				<Tooltip>
					<TooltipTrigger asChild>
						<div>
							<AvatarData
								title={
									<span className="block truncate">
										{user.name || user.username}
									</span>
								}
								subtitle={
									<span className="block truncate">@{user.username}</span>
								}
								src={user.avatar_url}
								imgFallbackText={user.username}
							/>
						</div>
					</TooltipTrigger>
					<TooltipContent>{user.name || user.username}</TooltipContent>
				</Tooltip>
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatCostMicros(user.total_cost_micros)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{user.message_count.toLocaleString()}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{user.chat_count.toLocaleString()}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.total_input_tokens)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.total_output_tokens)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.total_cache_read_tokens)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.total_cache_creation_tokens)}
			</TableCell>
		</TableRow>
	);
};
