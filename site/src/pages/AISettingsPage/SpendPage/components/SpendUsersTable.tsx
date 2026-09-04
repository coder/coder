import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import {
	Link as RouterLink,
	type To,
	useNavigate,
	useSearchParams,
} from "react-router";
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
import { formatTokenCount } from "#/utils/analytics";
import { formatCostMicros } from "#/utils/currency";
import type { SpendUsersQuery } from "../SpendPageView";
import { RetentionNotice } from "./RetentionNotice";
import { SpendSectionHeader } from "./SpendSectionHeader";

export const userSearchParam = "user";

interface SpendUsersTableProps {
	displayDateRange: DateRangeValue;
	onDateRangeChange: (value: DateRangeValue) => void;
	searchFilter: string;
	onSearchFilterChange: (value: string) => void;
	usersQuery: SpendUsersQuery;
}

export const SpendUsersTable: FC<SpendUsersTableProps> = ({
	displayDateRange,
	onDateRangeChange,
	searchFilter,
	onSearchFilterChange,
	usersQuery,
}) => {
	const [searchParams] = useSearchParams();
	const userDetailsTo = (user: TypesGen.AIGatewaySpendUser): To => {
		const next = new URLSearchParams(searchParams);
		next.set(userSearchParam, user.id);
		return { search: next.toString() };
	};
	const retryButton = (
		<Button
			variant="outline"
			size="sm"
			type="button"
			onClick={() => void usersQuery.refetch()}
		>
			Retry
		</Button>
	);

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
			{usersQuery.error != null && !usersQuery.data && (
				<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
					<ErrorAlert error={usersQuery.error} />
					{retryButton}
				</div>
			)}
			{usersQuery.data && (
				<>
					<RetentionNotice
						requestedStart={displayDateRange.startDate}
						appliedStart={usersQuery.data.start_date}
					/>
					{usersQuery.error != null && (
						<ErrorAlert error={usersQuery.error} actions={retryButton} />
					)}
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
								{searchFilter
									? "No users match this search."
									: "No AI Gateway spend for this period."}
							</p>
						) : (
							<PaginationContainer
								query={usersQuery}
								paginationUnitLabel="users"
							>
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
												<TableHead className="text-right">
													Cache write
												</TableHead>
											</TableRow>
										</TableHeader>
										<TableBody>
											{usersQuery.data.users.map((user) => (
												<UserRow
													key={user.id}
													user={user}
													detailsTo={userDetailsTo(user)}
												/>
											))}
										</TableBody>
									</Table>
								</div>
							</PaginationContainer>
						)}
					</div>
				</>
			)}
		</section>
	);
};

const UserRow: FC<{
	user: TypesGen.AIGatewaySpendUser;
	detailsTo: To;
}> = ({ user, detailsTo }) => {
	const navigate = useNavigate();

	// The row keeps its native <tr> semantics so screen readers announce every
	// cell. The name link is the keyboard-reachable control; clicking elsewhere
	// on the row is a mouse shortcut to the same place.
	return (
		<TableRow hover className="text-xs" onClick={() => navigate(detailsTo)}>
			<TableCell className="max-w-[200px] px-3 py-2">
				<AvatarData
					truncate
					title={
						<RouterLink
							to={detailsTo}
							className="hover:underline"
							onClick={(event) => event.stopPropagation()}
						>
							{user.name || user.username}
						</RouterLink>
					}
					subtitle={`@${user.username}`}
					src={user.avatar_url}
					imgFallbackText={user.username}
				/>
			</TableCell>
			<TableCell className="text-right tabular-nums">
				{formatCostMicros(user.total_cost_micros)}
				{user.unpriced_request_count > 0 && (
					<span className="mt-0.5 flex items-center justify-end gap-1 text-content-warning">
						<TriangleAlertIcon aria-hidden className="size-icon-xs" />
						{user.unpriced_request_count.toLocaleString("en-US")} unpriced
					</span>
				)}
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
