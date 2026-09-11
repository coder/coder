import { ArrowDownIcon, ArrowUpDownIcon, ArrowUpIcon } from "lucide-react";
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
import type { DateRangeValue } from "#/components/DateRangePicker/DateRangePicker";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
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
import type { SpendUsersQuery } from "../SpendPageView";
import { CostCell } from "./CostCell";
import { RetentionNotice } from "./RetentionNotice";

export const userSearchParam = "user";

const spendSortColumns: {
	field: TypesGen.AIGatewaySpendSortBy;
	label: string;
}[] = [
	{ field: "username", label: "User" },
	{ field: "total_cost_micros", label: "Cost" },
	{ field: "request_count", label: "Requests" },
	{ field: "session_count", label: "Sessions" },
	{ field: "input_tokens", label: "Input" },
	{ field: "output_tokens", label: "Output" },
	{ field: "cache_read_input_tokens", label: "Cache read" },
	{ field: "cache_write_input_tokens", label: "Cache write" },
];

/** Resolves URL sorting to the supported server fields and defaults. */
export function spendUsersSort(params: URLSearchParams): {
	sort_by: TypesGen.AIGatewaySpendSortBy;
	sort_order: TypesGen.AIGatewaySpendSortOrder;
} {
	return {
		sort_by:
			spendSortColumns.find(({ field }) => field === params.get("sort_by"))
				?.field ?? "total_cost_micros",
		sort_order: params.get("sort_order") === "asc" ? "asc" : "desc",
	};
}

// The list's query string travels in the drill-in's location state so Back
// can tell whether the history entry beneath it is the list it would show.
export const spendListSearchFromState = (state: unknown): string | null =>
	typeof state === "object" &&
	state !== null &&
	"fromSpendList" in state &&
	typeof state.fromSpendList === "string"
		? state.fromSpendList
		: null;

interface SpendUsersTableProps {
	displayDateRange: DateRangeValue;
	searchFilter: string;
	usersQuery: SpendUsersQuery;
}

export const SpendUsersTable: FC<SpendUsersTableProps> = ({
	displayDateRange,
	searchFilter,
	usersQuery,
}) => {
	const [searchParams, setSearchParams] = useSearchParams();
	const sort = spendUsersSort(searchParams);
	const onSort = (field: TypesGen.AIGatewaySpendSortBy) => {
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				next.set("sort_by", field);
				next.set(
					"sort_order",
					field === sort.sort_by
						? sort.sort_order === "asc"
							? "desc"
							: "asc"
						: field === "username"
							? "asc"
							: "desc",
				);
				next.delete("page");
				return next;
			},
			{ replace: true },
		);
	};
	const detailsState = { fromSpendList: searchParams.toString() };
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
		<div className="space-y-6">
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
						applied={usersQuery.data}
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
									: "No AI Gateway spend matches these filters."}
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
												{spendSortColumns.map(({ field, label }) => {
													const active = sort.sort_by === field;
													const Icon = active
														? sort.sort_order === "asc"
															? ArrowUpIcon
															: ArrowDownIcon
														: ArrowUpDownIcon;
													return (
														<TableHead
															key={field}
															className={
																field === "username" ? "" : "text-right"
															}
															aria-sort={
																active
																	? sort.sort_order === "asc"
																		? "ascending"
																		: "descending"
																	: "none"
															}
														>
															<Button
																variant="subtle"
																size="sm"
																className="px-0"
																onClick={() => onSort(field)}
															>
																{label}
																<Icon aria-hidden="true" className="size-3" />
															</Button>
														</TableHead>
													);
												})}
											</TableRow>
										</TableHeader>
										<TableBody>
											{usersQuery.data.users.map((user) => (
												<UserRow
													key={user.id}
													user={user}
													detailsTo={userDetailsTo(user)}
													detailsState={detailsState}
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
		</div>
	);
};

const UserRow: FC<{
	user: TypesGen.AIGatewaySpendUser;
	detailsTo: To;
	detailsState: { fromSpendList: string };
}> = ({ user, detailsTo, detailsState }) => {
	const navigate = useNavigate();

	// The row keeps its native <tr> semantics so screen readers announce every
	// cell. The name link is the keyboard-reachable control; clicking elsewhere
	// on the row is a mouse shortcut to the same place.
	return (
		<TableRow
			hover
			className="text-xs"
			onClick={() => navigate(detailsTo, { state: detailsState })}
		>
			<TableCell className="max-w-[200px] px-3 py-2">
				<AvatarData
					truncate
					title={
						<RouterLink
							to={detailsTo}
							state={detailsState}
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
			<CostCell
				costMicros={user.total_cost_micros}
				unpricedRequestCount={user.unpriced_request_count}
			/>
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
