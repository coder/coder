import type { FC, ReactNode } from "react";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "#/components/Chart/Chart";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import { Loader } from "#/components/Loader/Loader";
import { PaginationContainer } from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import type { UsePaginatedQueryResult } from "#/hooks/usePaginatedQuery";
import {
	addTime,
	durationInHours,
	formatDate,
	formatDateTime,
	startOfDay,
	startOfHour,
	subtractTime,
} from "#/utils/time";

export type AgentRuntimeRange = Readonly<{
	startTime: string;
	endTime: string;
}>;

// The backend parses start_time/end_time as RFC3339 and validates the
// clock in whatever offset was sent, so these must carry the local UTC
// offset rather than being converted to "Z" (which shifts local midnight
// to a non-zero UTC hour and fails validation). Mirrors
// TemplateInsightsPage's toISOLocal.
const RFC3339_LOCAL_FORMAT = "YYYY-MM-DDTHH:mm:ssZ";
export const toRFC3339Local = (date: Date): string =>
	formatDateTime(date, RFC3339_LOCAL_FORMAT);

// Mirrors DateRangePicker's own boundary normalization (toBoundary): a
// midnight-aligned start and an end that is either the next midnight or,
// for a range ending today, the next hour boundary. The backend rejects
// start_time/end_time values that aren't aligned this way.
export const DEFAULT_RANGE: AgentRuntimeRange = {
	startTime: toRFC3339Local(startOfDay(subtractTime(new Date(), 30, "day"))),
	endTime: toRFC3339Local(addTime(startOfHour(new Date()), 1, "hour")),
};

export const rangeFromSearchParams = (
	searchParams: URLSearchParams,
): AgentRuntimeRange | undefined => {
	const startDate = searchParams.get("startDate");
	const endDate = searchParams.get("endDate");
	if (!startDate || !endDate) {
		return undefined;
	}
	const start = new Date(startDate);
	const end = new Date(endDate);
	if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) {
		return undefined;
	}
	// Re-normalize instead of passing raw URL values through: stale or
	// hand-edited params may not satisfy the backend's boundary-alignment
	// rules. Normalization is idempotent for values that already comply.
	const now = new Date();
	const sameDayAsNow = startOfDay(end).getTime() === startOfDay(now).getTime();
	return {
		startTime: toRFC3339Local(startOfDay(start)),
		endTime: sameDayAsNow
			? toRFC3339Local(addTime(startOfHour(now), 1, "hour"))
			: toRFC3339Local(startOfDay(end)),
	};
};

const chartConfig = {
	hours: {
		label: "Agent hours",
		color: "hsl(var(--highlight-purple))",
	},
} satisfies ChartConfig;

// Agent hours rounded to one decimal place, e.g. "12.3 hours".
const formatAgentHours = (ms: number): string =>
	`${durationInHours(ms).toFixed(1)} hours`;

// Hour-and-minute granularity with no smaller units, e.g.
// "5 hours 15 minutes".
const formatHoursMinutes = (ms: number): string => {
	const totalMinutes = Math.round(ms / 60_000);
	const hours = Math.floor(totalMinutes / 60);
	const minutes = totalMinutes % 60;
	const hoursPart = `${hours} ${hours === 1 ? "hour" : "hours"}`;
	const minutesPart = `${minutes} ${minutes === 1 ? "minute" : "minutes"}`;
	if (hours === 0) {
		return minutesPart;
	}
	if (minutes === 0) {
		return hoursPart;
	}
	return `${hoursPart} ${minutesPart}`;
};

type SortColumn = "username" | "totalMs" | "messageCount";
type SortDirection = "asc" | "desc";

export interface CoderAgentsUsagePageViewProps {
	range: AgentRuntimeRange;
	onRangeChange: (range: AgentRuntimeRange) => void;
	summaryData: TypesGen.AgentRuntimeInsightsResponse | undefined;
	summaryError: unknown;
	isLoadingSummary: boolean;
	usersQuery: UsePaginatedQueryResult<TypesGen.AgentRuntimeInsightsByUserResponse>;
	sortColumn?: SortColumn;
	sortDirection?: SortDirection;
	onSortChange?: (column: SortColumn, direction: SortDirection) => void;
}

const SortableHeader: FC<{
	column: SortColumn;
	label: ReactNode;
	sortColumn: SortColumn | undefined;
	sortDirection: SortDirection | undefined;
	onSortChange?: (column: SortColumn, direction: SortDirection) => void;
	className?: string;
}> = ({
	column,
	label,
	sortColumn,
	sortDirection,
	onSortChange,
	className,
}) => {
	const isActive = sortColumn === column;
	return (
		<TableHead
			className={className}
			aria-sort={
				isActive
					? sortDirection === "asc"
						? "ascending"
						: "descending"
					: "none"
			}
		>
			<button
				type="button"
				className="flex items-center gap-1 bg-transparent border-0 p-0 m-0 font-medium text-content-secondary hover:text-content-primary cursor-pointer"
				onClick={() =>
					onSortChange?.(
						column,
						isActive && sortDirection === "desc" ? "asc" : "desc",
					)
				}
			>
				{label}
				{isActive && (
					<span aria-hidden>{sortDirection === "asc" ? "▲" : "▼"}</span>
				)}
			</button>
		</TableHead>
	);
};

export const CoderAgentsUsagePageView: FC<CoderAgentsUsagePageViewProps> = ({
	range,
	onRangeChange,
	summaryData,
	summaryError,
	isLoadingSummary,
	usersQuery,
	sortColumn,
	sortDirection,
	onSortChange,
}) => {
	const dateRangeValue: DateRangeValue = {
		startDate: new Date(range.startTime),
		endDate: new Date(range.endTime),
	};

	const chartData = summaryData?.by_day.map((entry) => ({
		date: entry.day,
		hours: Number(durationInHours(entry.total_ms).toFixed(1)),
	}));

	const users = usersQuery.data?.users ?? [];
	const sortedUsers = sortColumn
		? [...users].sort((a, b) => {
				let cmp = 0;
				if (sortColumn === "username") {
					cmp = a.username.localeCompare(b.username);
				} else if (sortColumn === "totalMs") {
					cmp = a.total_ms - b.total_ms;
				} else {
					cmp = a.message_count - b.message_count;
				}
				return sortDirection === "asc" ? cmp : -cmp;
			})
		: users;

	return (
		<>
			<SettingsHeader>
				<SettingsHeaderTitle>Coder Agents Usage</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Total agent working hours for the deployment, by user, over the
					selected date range.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<div className="flex flex-col gap-4">
				<div className="flex items-center justify-between">
					<div>
						<div className="text-xs text-content-secondary font-medium">
							Total agent hours
						</div>
						<div className="text-2xl font-semibold">
							{isLoadingSummary
								? "…"
								: summaryData
									? formatAgentHours(summaryData.total_ms)
									: "—"}
						</div>
					</div>
					<DateRangePicker
						value={dateRangeValue}
						onChange={(value) =>
							onRangeChange({
								startTime: toRFC3339Local(value.startDate),
								endTime: toRFC3339Local(value.endDate),
							})
						}
					/>
				</div>

				{summaryError ? (
					<ErrorAlert error={summaryError} />
				) : (
					<div className="border border-solid rounded p-4">
						<div className="h-64">
							{isLoadingSummary ? (
								<Loader />
							) : chartData && chartData.length > 0 ? (
								<ChartContainer
									config={chartConfig}
									className="aspect-auto h-full"
								>
									<AreaChart
										accessibilityLayer
										data={chartData}
										margin={{ top: 10, left: 0, right: 0 }}
									>
										<CartesianGrid vertical={false} />
										<XAxis
											dataKey="date"
											tickLine={false}
											tickMargin={12}
											minTickGap={24}
											tickFormatter={(value: string) =>
												formatDate(new Date(value), {
													month: "short",
													day: "numeric",
													year: undefined,
													hour: undefined,
													minute: undefined,
													second: undefined,
												})
											}
										/>
										<YAxis
											dataKey="hours"
											tickLine={false}
											axisLine={false}
											tickMargin={12}
											tickFormatter={(value: number) =>
												value === 0 ? "" : value.toLocaleString()
											}
										/>
										<ChartTooltip
											cursor={false}
											content={
												<ChartTooltipContent
													className="font-medium text-content-secondary"
													labelClassName="text-content-primary"
													formatter={(value) => (
														<div className="flex flex-1 items-center justify-between gap-2">
															<span className="text-content-secondary">
																Agent hours
															</span>
															<span className="font-mono font-medium tabular-nums text-content-primary">
																{value}
															</span>
														</div>
													)}
													labelFormatter={(_, p) => {
														const item = p[0];
														return formatDate(new Date(item.payload.date), {
															month: "short",
															day: "numeric",
															year: "numeric",
															hour: undefined,
															minute: undefined,
															second: undefined,
														});
													}}
												/>
											}
										/>
										<Area
											dataKey="hours"
											type="natural"
											fill="var(--color-hours)"
											fillOpacity={0.2}
											stroke="var(--color-hours)"
										/>
									</AreaChart>
								</ChartContainer>
							) : (
								<div className="flex items-center justify-center h-full text-content-secondary text-sm">
									No agent usage in this range.
								</div>
							)}
						</div>
					</div>
				)}

				{usersQuery.error ? (
					<ErrorAlert error={usersQuery.error} />
				) : (
					<PaginationContainer query={usersQuery} paginationUnitLabel="users">
						<Table>
							<TableHeader>
								<TableRow>
									<SortableHeader
										column="username"
										label="User"
										sortColumn={sortColumn}
										sortDirection={sortDirection}
										onSortChange={onSortChange}
									/>
									<SortableHeader
										column="totalMs"
										label="Total agent hours"
										sortColumn={sortColumn}
										sortDirection={sortDirection}
										onSortChange={onSortChange}
									/>
									<SortableHeader
										column="messageCount"
										label="Messages"
										sortColumn={sortColumn}
										sortDirection={sortDirection}
										onSortChange={onSortChange}
									/>
								</TableRow>
							</TableHeader>
							<TableBody>
								{usersQuery.isLoading ? (
									<TableRow>
										<TableCell colSpan={3}>
											<Loader />
										</TableCell>
									</TableRow>
								) : sortedUsers.length === 0 ? (
									<TableRow>
										<TableCell colSpan={3} className="text-content-secondary">
											No agent usage in this range.
										</TableCell>
									</TableRow>
								) : (
									sortedUsers.map((user) => (
										<TableRow key={user.user_id}>
											<TableCell>
												<div className="flex items-center gap-2">
													<Avatar
														src={user.avatar_url}
														fallback={user.username}
														size="sm"
													/>
													{user.username}
												</div>
											</TableCell>
											<TableCell>{formatHoursMinutes(user.total_ms)}</TableCell>
											<TableCell>
												{user.message_count.toLocaleString()}
											</TableCell>
										</TableRow>
									))
								)}
							</TableBody>
						</Table>
					</PaginationContainer>
				)}
			</div>
		</>
	);
};

export default CoderAgentsUsagePageView;
