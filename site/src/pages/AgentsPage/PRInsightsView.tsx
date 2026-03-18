import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "components/Chart/Chart";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import {
	ArrowDownRightIcon,
	ArrowUpRightIcon,
	CheckCircle2Icon,
	CircleDotIcon,
	CodeIcon,
	ExternalLinkIcon,
	MessageSquareTextIcon,
} from "lucide-react";
import type { FC } from "react";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { cn } from "utils/cn";
import { formatCostMicros } from "utils/currency";
import { DiffStatBadge } from "./DiffStats";
import { PrStateIcon } from "./GitPanel";

dayjs.extend(relativeTime);

// ---------------------------------------------------------------------------
// Component props
// ---------------------------------------------------------------------------

export type PRInsightsTimeRange = "7d" | "14d" | "30d" | "90d";

interface PRInsightsViewProps {
	data: TypesGen.PRInsightsResponse;
	timeRange: PRInsightsTimeRange;
	onTimeRangeChange: (range: PRInsightsTimeRange) => void;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function pctChange(current: number, previous: number): number | null {
	if (previous === 0) return current > 0 ? 100 : null;
	return ((current - previous) / previous) * 100;
}

function formatPct(value: number): string {
	return `${value >= 0 ? "+" : ""}${Math.round(value)}%`;
}

function formatMergeRate(rate: number): string {
	return `${Math.round(rate * 100)}%`;
}

function formatLinesShipped(n: number): string {
	if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
	if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
	return n.toLocaleString();
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

const TrendBadge: FC<{
	current: number;
	previous: number;
	invert?: boolean;
}> = ({ current, previous, invert = false }) => {
	const change = pctChange(current, previous);
	if (change === null) return null;

	const isPositive = invert ? change < 0 : change > 0;
	const isNegative = invert ? change > 0 : change < 0;

	if (isPositive) {
		return (
			<span className="inline-flex items-center gap-0.5 rounded-md bg-surface-green px-1.5 py-0.5 text-[11px] font-medium leading-none text-content-success">
				<ArrowUpRightIcon className="size-3" />
				{formatPct(change)}
			</span>
		);
	}
	if (isNegative) {
		return (
			<span className="inline-flex items-center gap-0.5 rounded-md bg-surface-red px-1.5 py-0.5 text-[11px] font-medium leading-none text-content-destructive">
				<ArrowDownRightIcon className="size-3" />
				{formatPct(change)}
			</span>
		);
	}
	return (
		<span className="inline-flex items-center rounded-md bg-surface-tertiary px-1.5 py-0.5 text-[11px] font-medium leading-none text-content-secondary">
			0%
		</span>
	);
};

const StatCard: FC<{
	label: string;
	value: string;
	trend?: React.ReactNode;
	detail?: string;
}> = ({ label, value, trend, detail }) => (
	<div className="flex flex-col justify-between rounded-lg border border-border-default bg-surface-primary p-5">
		<p className="m-0 text-[13px] text-content-secondary">{label}</p>
		<div className="mt-2">
			<div className="flex items-baseline gap-2">
				<p className="m-0 text-[28px] font-semibold leading-none tracking-tight text-content-primary">
					{value}
				</p>
				{trend}
			</div>
			{detail && (
				<p className="m-0 mt-1.5 text-xs text-content-disabled">{detail}</p>
			)}
		</div>
	</div>
);

const prStateBadgeStyles: Record<string, string> = {
	merged: "text-git-merged-bright ring-current/20",
	closed: "text-git-deleted-bright ring-current/20",
	open: "text-git-added-bright ring-current/20",
	draft: "text-content-secondary ring-border-default",
};

const prStateLabels: Record<string, string> = {
	merged: "Merged",
	closed: "Closed",
	open: "Open",
	draft: "Draft",
};

function prStateKey(state: string, draft: boolean): string {
	if (state === "merged" || state === "closed") return state;
	return draft ? "draft" : "open";
}

const PRStateBadge: FC<{ state: string; draft: boolean }> = ({
	state,
	draft,
}) => {
	const key = prStateKey(state, draft);

	return (
		<span
			className={cn(
				"inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset",
				prStateBadgeStyles[key] ?? prStateBadgeStyles.open,
			)}
		>
			<PrStateIcon state={state} draft={draft} className="size-3" />
			{prStateLabels[key] ?? "Open"}
		</span>
	);
};

const InlineMergeBar: FC<{ rate: number }> = ({ rate }) => (
	<div className="flex items-center gap-2.5">
		<div className="h-[6px] w-20 overflow-hidden rounded-full bg-surface-tertiary">
			<div
				className="h-full rounded-full bg-git-merged-bright transition-all"
				style={{ width: `${Math.round(rate * 100)}%` }}
			/>
		</div>
		<span className="w-8 text-right text-xs tabular-nums text-content-primary">
			{formatMergeRate(rate)}
		</span>
	</div>
);

// ---------------------------------------------------------------------------
// Chart configuration
// ---------------------------------------------------------------------------

const activityChartConfig = {
	prs_created: {
		label: "Created",
		color: "hsl(var(--git-added-bright))",
	},
	prs_merged: {
		label: "Merged",
		color: "hsl(var(--git-merged-bright))",
	},
	prs_closed: {
		label: "Closed",
		color: "hsl(var(--git-deleted-bright))",
	},
} satisfies ChartConfig;

function formatChartDate(dateStr: string): string {
	return dayjs(dateStr).format("MMM D");
}

// ---------------------------------------------------------------------------
// Activity chart
// ---------------------------------------------------------------------------

const ActivityChart: FC<{
	data: readonly TypesGen.PRInsightsTimeSeriesEntry[];
}> = ({ data }) => (
	<ChartContainer config={activityChartConfig} className="aspect-auto h-full">
		<AreaChart
			accessibilityLayer
			data={[...data]}
			margin={{ top: 8, left: -8, right: 8, bottom: 0 }}
		>
			<defs>
				<linearGradient id="fillCreated" x1="0" y1="0" x2="0" y2="1">
					<stop
						offset="5%"
						stopColor="var(--color-prs_created)"
						stopOpacity={0.35}
					/>
					<stop
						offset="95%"
						stopColor="var(--color-prs_created)"
						stopOpacity={0}
					/>
				</linearGradient>
				<linearGradient id="fillMerged" x1="0" y1="0" x2="0" y2="1">
					<stop
						offset="5%"
						stopColor="var(--color-prs_merged)"
						stopOpacity={0.5}
					/>
					<stop
						offset="95%"
						stopColor="var(--color-prs_merged)"
						stopOpacity={0.02}
					/>
				</linearGradient>
				<linearGradient id="fillClosed" x1="0" y1="0" x2="0" y2="1">
					<stop
						offset="5%"
						stopColor="var(--color-prs_closed)"
						stopOpacity={0.35}
					/>
					<stop
						offset="95%"
						stopColor="var(--color-prs_closed)"
						stopOpacity={0}
					/>
				</linearGradient>
			</defs>
			<CartesianGrid vertical={false} />
			<XAxis
				dataKey="date"
				tickLine={false}
				tickMargin={12}
				minTickGap={40}
				tickFormatter={formatChartDate}
			/>
			<YAxis
				tickLine={false}
				axisLine={false}
				tickMargin={12}
				allowDecimals={false}
				tickFormatter={(v: number) => (v === 0 ? "" : String(v))}
			/>
			<ChartTooltip
				cursor={false}
				content={
					<ChartTooltipContent
						labelFormatter={(v: string) => dayjs(v).format("ddd, MMM D")}
					/>
				}
			/>
			<Area
				isAnimationActive={false}
				type="monotone"
				dataKey="prs_created"
				fill="url(#fillCreated)"
				fillOpacity={1}
				stroke="var(--color-prs_created)"
				strokeWidth={1.5}
			/>
			<Area
				isAnimationActive={false}
				type="monotone"
				dataKey="prs_merged"
				fill="url(#fillMerged)"
				fillOpacity={1}
				stroke="var(--color-prs_merged)"
				strokeWidth={2}
			/>
			<Area
				isAnimationActive={false}
				type="monotone"
				dataKey="prs_closed"
				fill="url(#fillClosed)"
				fillOpacity={1}
				stroke="var(--color-prs_closed)"
				strokeWidth={1.5}
			/>
		</AreaChart>
	</ChartContainer>
);

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

const EmptyState: FC = () => (
	<div className="flex flex-col items-center justify-center gap-3 py-20 text-center">
		<div className="flex size-12 items-center justify-center rounded-full bg-surface-secondary">
			<CodeIcon className="size-5 text-content-secondary" />
		</div>
		<div>
			<p className="m-0 text-sm font-medium text-content-primary">
				No pull requests yet
			</p>
			<p className="m-0 mt-1 text-sm text-content-secondary">
				Pull request data will appear here once agents start shipping code.
			</p>
		</div>
	</div>
);

// ---------------------------------------------------------------------------
// Section header helper
// ---------------------------------------------------------------------------

const SectionTitle: FC<{ children: string }> = ({ children }) => (
	<h3 className="m-0 text-sm font-medium text-content-primary">{children}</h3>
);

const timeRangeOptions: { value: PRInsightsTimeRange; label: string }[] = [
	{ value: "7d", label: "7d" },
	{ value: "14d", label: "14d" },
	{ value: "30d", label: "30d" },
	{ value: "90d", label: "90d" },
];

const TimeRangeFilter: FC<{
	value: PRInsightsTimeRange;
	onChange: (range: PRInsightsTimeRange) => void;
}> = ({ value, onChange }) => (
	<div className="inline-flex -space-x-px">
		{timeRangeOptions.map((opt, i) => (
			<Button
				key={opt.value}
				variant={value === opt.value ? "default" : "outline"}
				size="sm"
				onClick={() => onChange(opt.value)}
				className={cn(
					"min-w-0 rounded-none px-3",
					i === 0 && "rounded-l-md",
					i === timeRangeOptions.length - 1 && "rounded-r-md",
				)}
			>
				{opt.label}
			</Button>
		))}
	</div>
);

const ReviewBadge: FC<{
	approved: boolean | undefined;
	changes_requested: boolean;
	reviewer_count: number | undefined;
}> = ({ approved, changes_requested, reviewer_count }) => {
	if (!reviewer_count) {
		return <span className="text-xs text-content-disabled">No reviews</span>;
	}

	if (approved === true && !changes_requested) {
		return (
			<span className="inline-flex items-center gap-1 text-xs font-medium text-content-success">
				<CheckCircle2Icon className="size-3.5" />
				{reviewer_count} approved
			</span>
		);
	}

	if (changes_requested) {
		return (
			<span className="inline-flex items-center gap-1 text-xs font-medium text-content-warning">
				<MessageSquareTextIcon className="size-3.5" />
				Changes requested
			</span>
		);
	}

	return (
		<span className="inline-flex items-center gap-1 text-xs text-content-secondary">
			<CircleDotIcon className="size-3.5" />
			{reviewer_count} reviewing
		</span>
	);
};

// ---------------------------------------------------------------------------
// Main view
// ---------------------------------------------------------------------------

export const PRInsightsView: FC<PRInsightsViewProps> = ({
	data,
	timeRange,
	onTimeRangeChange,
}) => {
	const { summary, time_series, by_model, recent_prs } = data;
	const isEmpty = summary.total_prs_created === 0;

	return (
		<div className="space-y-10">
			{/* ── Header ── */}
			<div className="flex items-end justify-between">
				<div>
					<h2 className="m-0 text-xl font-semibold tracking-tight text-content-primary">
						Pull Request Insights
					</h2>
					<p className="m-0 mt-1 text-[13px] text-content-secondary">
						Code shipped by AI agents across your organization.
					</p>
				</div>
				<TimeRangeFilter value={timeRange} onChange={onTimeRangeChange} />
			</div>

			{isEmpty ? (
				<EmptyState />
			) : (
				<>
					{/* ── Stat cards ── */}
					<div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
						<StatCard
							label="PRs created"
							value={summary.total_prs_created.toLocaleString()}
							trend={
								<TrendBadge
									current={summary.total_prs_created}
									previous={summary.prev_total_prs_created}
								/>
							}
						/>
						<StatCard
							label="Merged"
							value={summary.total_prs_merged.toLocaleString()}
							trend={
								<TrendBadge
									current={summary.total_prs_merged}
									previous={summary.prev_total_prs_merged}
								/>
							}
						/>
						<StatCard
							label="Merge rate"
							value={formatMergeRate(summary.merge_rate)}
							trend={
								<TrendBadge
									current={summary.merge_rate}
									previous={summary.prev_merge_rate}
								/>
							}
						/>
						<StatCard
							label="Lines shipped"
							value={formatLinesShipped(summary.total_additions)}
							detail={`${formatLinesShipped(summary.total_deletions)} removed`}
						/>
						<StatCard
							label="Cost / merged PR"
							value={formatCostMicros(summary.cost_per_merged_pr_micros)}
							trend={
								<TrendBadge
									current={summary.cost_per_merged_pr_micros}
									previous={summary.prev_cost_per_merged_pr_micros}
									invert
								/>
							}
						/>
					</div>

					{/* ── Activity chart ── */}
					<section>
						<div className="mb-4 flex items-center justify-between">
							<SectionTitle>Activity</SectionTitle>
							<div className="flex items-center gap-5">
								{Object.entries(activityChartConfig).map(([key, cfg]) => (
									<div key={key} className="flex items-center gap-1.5">
										<span
											className="inline-block size-2 rounded-full"
											style={{ backgroundColor: cfg.color }}
										/>
										<span className="text-xs text-content-secondary">
											{cfg.label}
										</span>
									</div>
								))}
							</div>
						</div>
						<div className="h-[260px] rounded-lg border border-border-default p-4 pt-2">
							<ActivityChart data={time_series} />
						</div>
					</section>

					{/* ── Model performance ── */}
					{by_model.length > 0 && (
						<section>
							<div className="mb-4">
								<SectionTitle>Performance by model</SectionTitle>
							</div>
							<div className="overflow-hidden rounded-lg border border-border-default">
								<Table className="text-sm">
									<TableHeader>
										<TableRow className="text-left text-xs text-content-secondary [&>th]:font-normal">
											<TableHead className="px-4 py-3">Model</TableHead>
											<TableHead className="px-4 py-3 text-right">
												PRs
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Merged
											</TableHead>
											<TableHead className="px-4 py-3">Merge rate</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Changes
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Total cost
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Cost / merge
											</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{by_model.map((m) => (
											<TableRow
												key={m.model_config_id}
												className="border-t border-border-default"
											>
												<TableCell className="px-4 py-3">
													<span className="font-medium text-content-primary">
														{m.display_name}
													</span>
													<span className="ml-1.5 text-xs text-content-disabled">
														{m.provider}
													</span>
												</TableCell>
												<TableCell className="px-4 py-3 text-right tabular-nums text-content-primary">
													{m.total_prs}
												</TableCell>
												<TableCell className="px-4 py-3 text-right tabular-nums text-content-primary">
													{m.merged_prs}
												</TableCell>
												<TableCell className="px-4 py-3">
													<InlineMergeBar rate={m.merge_rate} />
												</TableCell>
												<TableCell className="px-4 py-3 text-right">
													<DiffStatBadge
														additions={m.total_additions}
														deletions={m.total_deletions}
													/>
												</TableCell>
												<TableCell className="px-4 py-3 text-right tabular-nums text-content-secondary">
													{formatCostMicros(m.total_cost_micros)}
												</TableCell>
												<TableCell className="px-4 py-3 text-right tabular-nums text-content-primary">
													{m.merged_prs > 0
														? formatCostMicros(m.cost_per_merged_pr_micros)
														: "—"}
												</TableCell>
											</TableRow>
										))}
									</TableBody>
								</Table>
							</div>
						</section>
					)}

					{/* ── Recent pull requests ── */}
					{recent_prs.length > 0 && (
						<section>
							<div className="mb-4">
								<SectionTitle>Recent pull requests</SectionTitle>
							</div>
							<div className="overflow-hidden rounded-lg border border-border-default">
								<Table className="text-sm">
									<TableHeader>
										<TableRow className="text-left text-xs text-content-secondary [&>th]:font-normal">
											<TableHead className="px-4 py-3">Pull request</TableHead>
											<TableHead className="px-4 py-3">Status</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Changes
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Reviews
											</TableHead>
											<TableHead className="px-4 py-3">Model</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Cost
											</TableHead>
											<TableHead className="px-4 py-3 text-right">
												Created
											</TableHead>
										</TableRow>
									</TableHeader>
									<TableBody>
										{recent_prs.map((pr) => (
											<TableRow
												key={pr.chat_id}
												className="border-t border-border-default transition-colors hover:bg-surface-secondary/50"
											>
												<TableCell className="max-w-[320px] px-4 py-3">
													<a
														href={pr.pr_url}
														target="_blank"
														rel="noopener noreferrer"
														className="group flex items-start gap-1 text-sm font-medium text-content-primary no-underline hover:text-content-link"
													>
														<span className="truncate">{pr.pr_title}</span>
														<ExternalLinkIcon className="mt-0.5 size-3 shrink-0 text-content-disabled opacity-0 transition-opacity group-hover:opacity-100" />
													</a>
													<div className="mt-1 flex items-center gap-1.5 text-xs text-content-disabled">
														<img
															src={pr.author_avatar_url}
															alt=""
															className="size-3.5 rounded-full"
														/>
														<span>{pr.author_login}</span>
														<span>·</span>
														<span className="font-mono">#{pr.pr_number}</span>
														<span>→</span>
														<span className="font-mono">{pr.base_branch}</span>
													</div>
												</TableCell>
												<TableCell className="px-4 py-3">
													<PRStateBadge state={pr.state} draft={pr.draft} />
												</TableCell>
												<TableCell className="px-4 py-3 text-right">
													<DiffStatBadge
														additions={pr.additions}
														deletions={pr.deletions}
													/>
													<p className="m-0 mt-1 text-xs text-content-disabled">
														{pr.changed_files} file
														{pr.changed_files !== 1 ? "s" : ""}
													</p>
												</TableCell>
												<TableCell className="px-4 py-3 text-right">
													<ReviewBadge
														approved={pr.approved}
														changes_requested={pr.changes_requested}
														reviewer_count={pr.reviewer_count}
													/>
												</TableCell>{" "}
												<TableCell className="px-4 py-3 text-xs text-content-secondary">
													{pr.model_display_name}
												</TableCell>
												<TableCell className="px-4 py-3 text-right tabular-nums text-content-secondary">
													{formatCostMicros(pr.cost_micros)}
												</TableCell>
												<TableCell className="whitespace-nowrap px-4 py-3 text-right text-xs text-content-disabled">
													{dayjs(pr.created_at).format("MMM D, h:mm A")}
												</TableCell>
											</TableRow>
										))}
									</TableBody>
								</Table>
							</div>
						</section>
					)}
				</>
			)}
		</div>
	);
};
