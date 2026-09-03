import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
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
import type { SpendUsersQuery } from "../SpendPageView";
import { SpendSectionHeader } from "./SpendSectionHeader";
import { unpricedRequestsMessage } from "./unpricedRequests";

interface SpendUsersTableProps {
	displayDateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	searchFilter: string;
	onSearchFilterChange: (value: string) => void;
	usersQuery: SpendUsersQuery;
	onSelectUser: (user: TypesGen.AIGatewaySpendUser) => void;
}

export const SpendUsersTable: FC<SpendUsersTableProps> = ({
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
				title="Spend by user"
				description="AI Gateway cost and usage for each user in the selected date range."
				actions={
					<DateRangePicker
						value={displayDateRange}
						onChange={onDateRangeChange}
					/>
				}
			/>
			<div className="w-full md:max-w-sm">
				<SearchField
					value={searchFilter}
					onChange={onSearchFilterChange}
					placeholder="Search by name or username"
					aria-label="Search spend by name or username"
				/>
			</div>
			{usersQuery.isLoading && (
				<div
					role="status"
					aria-label="Loading spend"
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
							aria-label="Refreshing spend"
							className="absolute inset-0 z-10 flex items-center justify-center bg-surface-primary/50"
						>
							<Spinner size="lg" loading className="text-content-secondary" />
						</div>
					)}
					{usersQuery.data.users.length === 0 ? (
						<p className="py-12 text-center text-content-secondary">
							No AI Gateway spend for this period.
						</p>
					) : (
						<PaginationContainer query={usersQuery} paginationUnitLabel="users">
							<div className="overflow-hidden rounded-lg border border-border-default">
								<Table aria-label="Spend by user">
									<TableHeader>
										<TableRow>
											<TableHead>User</TableHead>
											<TableHead className="text-right">Cost</TableHead>
											<TableHead className="text-right">Requests</TableHead>
											<TableHead className="text-right">Sessions</TableHead>
											<TableHead className="text-right">Input</TableHead>
											<TableHead className="text-right">Output</TableHead>
											<TableHead className="text-right">Cache read</TableHead>
											<TableHead className="text-right">Cache write</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{usersQuery.data.users.map((user) => (
											<UserRow
												key={user.id}
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
	user: TypesGen.AIGatewaySpendUser;
	onSelect: (user: TypesGen.AIGatewaySpendUser) => void;
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
				<AvatarData
					title={
						<span className="block truncate">{user.name || user.username}</span>
					}
					subtitle={<span className="block truncate">@{user.username}</span>}
					src={user.avatar_url}
					imgFallbackText={user.username}
				/>
			</TableCell>
			<TableCell className="text-right tabular-nums">
				<span className="inline-flex items-center justify-end gap-1">
					{user.unpriced_request_count > 0 && (
						<Tooltip>
							<TooltipTrigger asChild>
								<TriangleAlertIcon
									aria-label={unpricedRequestsMessage(
										user.unpriced_request_count,
									)}
									className="size-icon-xs text-content-warning"
								/>
							</TooltipTrigger>
							<TooltipContent>
								{unpricedRequestsMessage(user.unpriced_request_count)}
							</TooltipContent>
						</Tooltip>
					)}
					{formatCostMicros(user.total_cost_micros)}
				</span>
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{user.request_count.toLocaleString("en-US")}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{user.session_count.toLocaleString("en-US")}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.input_tokens)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.output_tokens)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.cache_read_input_tokens)}
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatTokenCount(user.cache_write_input_tokens)}
			</TableCell>
		</TableRow>
	);
};
