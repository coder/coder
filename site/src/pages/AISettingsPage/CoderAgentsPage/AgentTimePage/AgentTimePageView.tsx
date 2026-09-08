import type { ComponentProps, FC } from "react";
import { getErrorMessage } from "#/api/errors";
import type { AgentTimeInterval, AgentTimeReport } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { BreakdownTable } from "./AgentTimeBreakdownTable";
import { AgentTimeChart } from "./AgentTimeChart";
import {
	type AgentTimeDatePreset,
	type AgentTimeSortBy,
	type AgentTimeSortOrder,
	type AgentTimeTableGroup,
	approximateBucketCount,
	dateOnlyToLocalDate,
	datePresetOptions,
	entityHeading,
	entityLabel,
	formatAgentTimeHours,
	formatDate,
	formatProcessedMessages,
	formatRange,
	inclusiveLocalDateFromExclusiveEnd,
	intervalLabel,
	intervalOptions,
	isAgentTimeInterval,
	localDateToDateOnly,
	parseAgentTimeMs,
	presetRange,
	shortId,
} from "./agentTimeUtils";

export type AgentTimeQuery = PaginationResult<AgentTimeReport> & {
	isLoading: boolean;
	isFetching: boolean;
	error: unknown;
	refetch: () => unknown;
};

interface AgentTimePageViewProps {
	query: AgentTimeQuery;
	now: Date;
	dateRange: DateRangeValue;
	activePreset: AgentTimeDatePreset;
	isAllHistory: boolean;
	endDate: string;
	interval: AgentTimeInterval;
	sortBy: AgentTimeSortBy;
	sortOrder: AgentTimeSortOrder;
	tableGroup: AgentTimeTableGroup;
	onGroupChange: (group: AgentTimeTableGroup) => void;
	selectedOrganizationId?: string;
	selectedUserId?: string;
	onDateRangeChange: (value: DateRangeValue) => void;
	onPresetChange: (preset: AgentTimeDatePreset) => void;
	onIntervalChange: (interval: AgentTimeInterval) => void;
	onSortChange: (sortBy: AgentTimeSortBy) => void;
	onSelectOrganization: (organizationId: string) => void;
	onClearOrganization: () => void;
	onSelectUser: (userId: string) => void;
	onClearUser: () => void;
	onRetry: () => void;
}

type DateRangePickerPreset = NonNullable<
	ComponentProps<typeof DateRangePicker>["presets"]
>[number];

function buildDateRangePickerPresets(now: Date): DateRangePickerPreset[] {
	const pickerOptions = datePresetOptions.filter(
		(
			option,
		): option is {
			value: Exclude<AgentTimeDatePreset, "custom" | "all_history">;
			label: string;
		} => option.value !== "all_history",
	);

	return pickerOptions.map((option) => ({
		label: option.label,
		range: () => {
			const range = presetRange(option.value, now);
			return {
				from: dateOnlyToLocalDate(range.startDate),
				to: inclusiveLocalDateFromExclusiveEnd(range.endDate),
			};
		},
	}));
}

function MetricCard({
	label,
	value,
	detail,
}: {
	label: string;
	value: string;
	detail?: string;
}) {
	return (
		<div className="rounded-lg border border-border-default bg-surface-primary p-4">
			<p className="m-0 text-xs font-medium text-content-secondary">{label}</p>
			<p className="m-0 mt-2 text-2xl font-semibold tracking-tight text-content-primary">
				{value}
			</p>
			{detail && (
				<p className="m-0 mt-1 text-xs text-content-disabled">{detail}</p>
			)}
		</div>
	);
}

function StatusAlerts({ report }: { report: AgentTimeReport }) {
	const hasPartialBuckets = report.buckets.some(
		(bucket) =>
			bucket.partial || !bucket.complete || bucket.agent_time_ms === null,
	);
	const hasBackfillError = report.status.backfill_error !== "";
	return (
		<div className="space-y-3">
			{report.historical_notice !== "" && (
				<Alert severity="info">
					<AlertTitle>Historical coverage</AlertTitle>
					<AlertDescription>{report.historical_notice}</AlertDescription>
				</Alert>
			)}
			{!report.status.backfill_complete && !hasBackfillError && (
				<Alert severity="info">
					<AlertTitle>Backfill in progress</AlertTitle>
					<AlertDescription>
						Processed{" "}
						{formatProcessedMessages(report.status.processed_messages)}
						messages. New agent time is captured while historical data is
						backfilled.
					</AlertDescription>
				</Alert>
			)}
			{hasBackfillError && (
				<Alert severity="error" prominent>
					<AlertTitle>Backfill failed</AlertTitle>
					<AlertDescription>{report.status.backfill_error}</AlertDescription>
				</Alert>
			)}
			{hasPartialBuckets && (
				<Alert severity="warning">
					<AlertTitle>Partial UTC coverage</AlertTitle>
					<AlertDescription>
						Some buckets are partial or unavailable. Empty buckets with
						unavailable agent time do not prove that no work occurred.
					</AlertDescription>
				</Alert>
			)}
		</div>
	);
}

function EmptyState({ tableGroup }: { tableGroup: AgentTimeTableGroup }) {
	return (
		<div className="rounded-lg border border-border-default px-6 py-16 text-center">
			<p className="m-0 text-sm font-medium text-content-primary">
				No agent time recorded
			</p>
			<p className="m-0 mt-1 text-sm text-content-secondary">
				Recorded agent time by {entityLabel(tableGroup)} will appear here once
				data is available for the selected UTC range.
			</p>
		</div>
	);
}

function LoadingState() {
	return (
		<div
			role="status"
			aria-label="Loading agent time"
			className="flex min-h-[320px] items-center justify-center rounded-lg border border-border-default"
		>
			<Spinner loading size="lg" className="text-content-secondary" />
		</div>
	);
}

function ErrorState({
	error,
	onRetry,
}: {
	error: unknown;
	onRetry: () => void;
}) {
	return (
		<div className="space-y-4">
			<Alert severity="error" prominent>
				<AlertTitle>
					{getErrorMessage(error, "Failed to load agent time.")}
				</AlertTitle>
				<AlertDescription>
					Check your permissions and try again. Agent time reporting requires
					deployment configuration read access.
				</AlertDescription>
			</Alert>
			<Button variant="outline" type="button" onClick={onRetry}>
				Retry
			</Button>
		</div>
	);
}

const AgentTimePageView: FC<AgentTimePageViewProps> = ({
	query,
	now,
	dateRange,
	activePreset: selectedPreset,
	isAllHistory,
	endDate,
	interval,
	sortBy,
	sortOrder,
	tableGroup,
	onGroupChange,
	selectedOrganizationId,
	selectedUserId,
	onDateRangeChange,
	onPresetChange,
	onIntervalChange,
	onSortChange,
	onSelectOrganization,
	onClearOrganization,
	onSelectUser,
	onClearUser,
	onRetry,
}) => {
	const report = query.data;
	const startDate = isAllHistory
		? undefined
		: localDateToDateOnly(dateRange.startDate);
	const pickerPresets = buildDateRangePickerPresets(now);
	const bucketCount = approximateBucketCount(interval, startDate, endDate);
	const selectedUserName = report?.rows.find(
		(row) => row.id === selectedUserId,
	)?.name;

	return (
		<div className="space-y-6">
			<SettingsHeader>
				<SettingsHeaderTitle>Agent time</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Review recorded Coder Agent time across UTC calendar ranges. Values
					come from persisted accounting aggregates, not raw message scans.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<section className="space-y-4 rounded-lg border border-border-default bg-surface-primary p-4">
				<div
					className="flex flex-wrap items-center gap-2"
					role="group"
					aria-label="Date presets"
				>
					{datePresetOptions.map((option) => (
						<Button
							key={option.value}
							variant={selectedPreset === option.value ? "default" : "outline"}
							size="sm"
							type="button"
							aria-pressed={selectedPreset === option.value}
							onClick={() => onPresetChange(option.value)}
						>
							{option.label}
						</Button>
					))}
					<Badge
						variant={selectedPreset === "custom" ? "info" : "default"}
						size="sm"
					>
						Custom
					</Badge>
				</div>
				<div className="flex flex-wrap items-end gap-4">
					<div className="flex flex-col gap-1">
						<span className="text-xs font-medium text-content-secondary">
							Custom range
						</span>
						<DateRangePicker
							value={dateRange}
							onChange={onDateRangeChange}
							now={now}
							presets={pickerPresets}
						/>
					</div>
					<div className="flex flex-col gap-1">
						<span className="text-xs font-medium text-content-secondary">
							Interval
						</span>
						<Select
							value={interval}
							onValueChange={(value) => {
								if (isAgentTimeInterval(value)) {
									onIntervalChange(value);
								}
							}}
						>
							<SelectTrigger className="w-36" aria-label="Chart interval">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{intervalOptions.map((option) => {
									const count = approximateBucketCount(
										option.value,
										startDate,
										endDate,
									);
									return (
										<SelectItem
											key={option.value}
											value={option.value}
											disabled={count !== undefined && count > 1000}
										>
											{option.label}
										</SelectItem>
									);
								})}
							</SelectContent>
						</Select>
					</div>
					<div className="text-xs text-content-secondary">
						<p className="m-0 font-medium text-content-primary">UTC range</p>
						<p className="m-0">{formatRange(startDate, endDate)}</p>
						{bucketCount !== undefined && (
							<p className="m-0">
								{bucketCount.toLocaleString("en-US")}{" "}
								{intervalLabel(interval).toLowerCase()} buckets
							</p>
						)}
					</div>
				</div>
			</section>

			<div role="group" aria-label="Breakdown dimension" className="flex gap-2">
				<Button
					variant={tableGroup === "organization" ? "default" : "outline"}
					aria-pressed={tableGroup === "organization"}
					onClick={() => onGroupChange("organization")}
				>
					Organizations
				</Button>
				<Button
					variant={tableGroup === "user" ? "default" : "outline"}
					aria-pressed={tableGroup === "user"}
					onClick={() => onGroupChange("user")}
				>
					Users
				</Button>
			</div>
			{!selectedOrganizationId && selectedUserId && (
				<Button variant="outline" onClick={onClearUser}>
					Clear user {selectedUserName || shortId(selectedUserId)}
				</Button>
			)}
			{selectedOrganizationId && (
				<div className="flex flex-wrap items-center gap-2 rounded-lg border border-border-default bg-surface-secondary p-3 text-sm">
					<span className="font-medium text-content-primary">
						Organization {shortId(selectedOrganizationId)}
					</span>
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={onClearOrganization}
					>
						All organizations
					</Button>
					{selectedUserId && (
						<Button
							variant="outline"
							size="sm"
							type="button"
							onClick={onClearUser}
						>
							Clear user {selectedUserName || shortId(selectedUserId)}
						</Button>
					)}
				</div>
			)}

			{query.error != null && (
				<ErrorState error={query.error} onRetry={onRetry} />
			)}
			{query.isLoading ? (
				<LoadingState />
			) : report ? (
				<div className="space-y-6">
					{query.isFetching && (
						<div role="status" className="text-sm text-content-secondary">
							Refreshing agent time...
						</div>
					)}
					<StatusAlerts report={report} />
					<div className="grid gap-3 md:grid-cols-3">
						<MetricCard
							label="Total agent time"
							value={formatAgentTimeHours(report.total_agent_time_ms)}
							detail={`${intervalLabel(report.interval)} buckets, UTC`}
						/>
						<MetricCard
							label={entityHeading(tableGroup)}
							value={report.count.toLocaleString("en-US")}
							detail="Total rows, independent of this page"
						/>
						<MetricCard
							label="Capture status"
							value={
								report.status.backfill_complete ? "Current" : "Backfilling"
							}
							detail={
								report.status.earliest_date
									? "Earliest aggregate " +
										formatDate(report.status.earliest_date)
									: "No aggregate date recorded"
							}
						/>
					</div>
					{parseAgentTimeMs(report.total_agent_time_ms) === 0n &&
					report.rows.length === 0 ? (
						<EmptyState tableGroup={tableGroup} />
					) : (
						<>
							<section className="space-y-3 rounded-lg border border-border-default bg-surface-primary p-4">
								<div className="flex items-center justify-between gap-3">
									<div>
										<h3 className="m-0 text-sm font-semibold text-content-primary">
											Agent time over time
										</h3>
										<p className="m-0 text-xs text-content-secondary">
											Buckets are UTC {report.interval} ranges. Unavailable
											buckets are left blank.
										</p>
									</div>
									<Badge variant="info" size="sm">
										UTC
									</Badge>
								</div>
								<AgentTimeChart report={report} />
							</section>
							<section className="space-y-3">
								<div>
									<h3 className="m-0 text-sm font-semibold text-content-primary">
										{entityHeading(tableGroup)}
									</h3>
									<p className="m-0 text-xs text-content-secondary">
										Ranked by recorded agent time for the selected UTC range.
									</p>
								</div>
								<BreakdownTable
									query={query}
									tableGroup={tableGroup}
									totalAgentTimeMs={report.total_agent_time_ms}
									sortBy={sortBy}
									sortOrder={sortOrder}
									selectedUserId={selectedUserId}
									onSortChange={onSortChange}
									onSelectOrganization={onSelectOrganization}
									onSelectUser={onSelectUser}
								/>
							</section>
						</>
					)}
				</div>
			) : null}
		</div>
	);
};

export default AgentTimePageView;
