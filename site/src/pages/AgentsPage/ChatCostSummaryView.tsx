import { getErrorMessage } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	type ChartConfig,
	ChartContainer,
	ChartTooltip,
	ChartTooltipContent,
} from "components/Chart/Chart";
import { Spinner } from "components/Spinner/Spinner";
import dayjs from "dayjs";
import type { FC } from "react";
import { Link } from "react-router";
import { Cell, Pie, PieChart } from "recharts";
import { formatTokenCount } from "utils/analytics";
import { formatCostMicros, microsToDollars } from "utils/currency";

interface ChatCostSummaryViewProps {
	summary: TypesGen.ChatCostSummary | undefined;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
	loadingLabel: string;
	emptyMessage: string;
}

const MODEL_COLORS = [
	"hsl(var(--highlight-purple))",
	"hsl(var(--highlight-sky))",
	"hsl(var(--highlight-green))",
	"hsl(var(--highlight-orange))",
	"hsl(var(--highlight-red))",
	"hsl(var(--highlight-magenta))",
] as const;

const SURFACE_COLORS = [
	"hsl(var(--surface-purple))",
	"hsl(var(--surface-sky))",
	"hsl(var(--surface-green))",
	"hsl(var(--surface-orange))",
	"hsl(var(--surface-red))",
	"hsl(var(--surface-magenta))",
] as const;

export const getUsageLimitPeriodLabel = (
	period: TypesGen.ChatUsageLimitPeriod | undefined,
): string => {
	switch (period) {
		case "day":
			return "Daily";
		case "week":
			return "Weekly";
		case "month":
			return "Monthly";
		default:
			return "";
	}
};

const formatBigTokens = (n: number): { short: string; full: string } => {
	const full = n.toLocaleString("en-US");
	if (n >= 1_000_000_000) {
		const b = n / 1_000_000_000;
		return { short: `${b % 1 === 0 ? b.toFixed(0) : b.toFixed(1)}B`, full };
	}
	if (n >= 1_000_000) {
		const m = n / 1_000_000;
		return { short: `${m % 1 === 0 ? m.toFixed(0) : m.toFixed(1)}M`, full };
	}
	if (n >= 1_000) {
		const k = n / 1_000;
		return { short: `${k % 1 === 0 ? k.toFixed(0) : k.toFixed(1)}K`, full };
	}
	return { short: String(n), full };
};

export const ChatCostSummaryView: FC<ChatCostSummaryViewProps> = ({
	summary,
	isLoading,
	error,
	onRetry,
	loadingLabel,
	emptyMessage,
}) => {
	if (isLoading) {
		return (
			<div
				role="status"
				aria-label={loadingLabel}
				className="flex min-h-[300px] items-center justify-center"
			>
				<Spinner size="lg" loading />
			</div>
		);
	}

	if (error != null) {
		return (
			<div className="flex min-h-[300px] flex-col items-center justify-center gap-3 text-center">
				<p className="m-0 text-sm text-content-secondary">
					{getErrorMessage(error, "Failed to load usage details.")}
				</p>
				<Button variant="outline" size="sm" type="button" onClick={onRetry}>
					Retry
				</Button>
			</div>
		);
	}

	if (!summary) {
		return null;
	}

	const totalMessages =
		summary.priced_message_count + summary.unpriced_message_count;
	const totalTokens =
		summary.total_input_tokens +
		summary.total_output_tokens +
		summary.total_cache_read_tokens +
		summary.total_cache_creation_tokens;
	const heroTokens = formatBigTokens(totalTokens);

	const usageLimit = summary.usage_limit;
	const showUsageLimit = usageLimit?.is_limited === true;
	const limitSpend = usageLimit?.current_spend ?? 0;
	const limitCap = usageLimit?.spend_limit_micros ?? 0;
	const limitPct =
		showUsageLimit && limitCap > 0
			? Math.min((limitSpend / limitCap) * 100, 100)
			: 0;
	const limitExceeded = showUsageLimit && limitSpend >= limitCap;
	const limitPeriodLabel = getUsageLimitPeriodLabel(usageLimit?.period);
	const limitResetAt =
		showUsageLimit && usageLimit?.period_end
			? dayjs(usageLimit.period_end).format("MMM D")
			: "";

	const ringRadius = 40;
	const ringCircumference = 2 * Math.PI * ringRadius;
	const ringOffset = ringCircumference * (1 - limitPct / 100);
	const ringColor = limitExceeded
		? "hsl(var(--highlight-red))"
		: limitPct >= 75
			? "hsl(var(--highlight-orange))"
			: "hsl(var(--highlight-green))";

	const sortedModels = [...summary.by_model].sort(
		(a, b) => b.total_cost_micros - a.total_cost_micros,
	);
	const totalModelCost = sortedModels.reduce(
		(sum, m) => sum + m.total_cost_micros,
		0,
	);
	const pieData = sortedModels.map((m) => ({
		name: m.display_name || m.model,
		value: microsToDollars(m.total_cost_micros),
	}));
	const chartConfig: ChartConfig = {};
	for (let i = 0; i < sortedModels.length; i++) {
		const m = sortedModels[i];
		chartConfig[m.model_config_id] = {
			label: m.display_name || m.model,
			color: MODEL_COLORS[i % MODEL_COLORS.length],
		};
	}

	const topChats = [...summary.by_chat]
		.sort((a, b) => b.total_cost_micros - a.total_cost_micros)
		.slice(0, 8);
	const maxChatCost = topChats.length > 0 ? topChats[0].total_cost_micros : 0;

	const hasBreakdownData =
		summary.by_model.length > 0 || summary.by_chat.length > 0;

	return (
		<div className="space-y-5">
			{/* Overview */}
			<div className="flex flex-col gap-6 pb-1 md:flex-row md:items-start md:justify-between">
				{/* Left — hero */}
				<div className="shrink-0">
					<p className="m-0 text-5xl font-bold leading-none tracking-tight text-content-primary">
						{heroTokens.short}{" "}
						<span className="text-2xl font-medium text-content-secondary">
							tokens
						</span>
					</p>
					<p className="m-0 mt-1 text-sm text-content-secondary">
						{totalMessages.toLocaleString()} messages ·{" "}
						{summary.by_chat.length.toLocaleString()} chats
					</p>
				</div>

				{/* Right — breakdowns + usage limit */}
				<div className="flex flex-col gap-3 md:items-end">
					<div className="grid grid-cols-2 gap-x-6 gap-y-1 text-sm">
						<div className="flex items-center justify-between gap-4">
							<span className="text-content-secondary">Input</span>
							<span className="font-medium text-content-primary">
								{formatTokenCount(summary.total_input_tokens)}
							</span>
						</div>
						<div className="flex items-center justify-between gap-4">
							<span className="text-content-secondary">Output</span>
							<span className="font-medium text-content-primary">
								{formatTokenCount(summary.total_output_tokens)}
							</span>
						</div>
						<div className="flex items-center justify-between gap-4">
							<span className="text-content-secondary">Cache read</span>
							<span className="font-medium text-content-primary">
								{formatTokenCount(summary.total_cache_read_tokens)}
							</span>
						</div>
						<div className="flex items-center justify-between gap-4">
							<span className="text-content-secondary">Cache write</span>
							<span className="font-medium text-content-primary">
								{formatTokenCount(summary.total_cache_creation_tokens)}
							</span>
						</div>
					</div>

					{showUsageLimit && usageLimit && (
						<div className="flex items-center gap-3 rounded-full bg-surface-secondary py-1.5 pr-4 pl-1.5">
							<div className="relative h-8 w-8 shrink-0">
								<svg viewBox="0 0 100 100" className="h-full w-full -rotate-90">
									<circle
										cx="50"
										cy="50"
										r={ringRadius}
										fill="none"
										stroke="hsl(var(--surface-tertiary))"
										strokeWidth="12"
									/>
									<circle
										cx="50"
										cy="50"
										r={ringRadius}
										fill="none"
										stroke={ringColor}
										strokeWidth="12"
										strokeLinecap="round"
										strokeDasharray={ringCircumference}
										strokeDashoffset={ringOffset}
										className="transition-[stroke-dashoffset] duration-500"
									/>
								</svg>
							</div>
							<div className="text-xs">
								<span className="font-medium text-content-primary">
									{formatCostMicros(limitSpend)}
								</span>
								<span className="text-content-secondary">
									{" "}/ {formatCostMicros(limitCap)}{" "}
									{limitPeriodLabel.toLowerCase()}
								</span>
								{limitResetAt && (
									<span className="text-content-secondary">
										{" "}· resets {limitResetAt}
									</span>
								)}
							</div>
						</div>
					)}
				</div>
			</div>

			{!hasBreakdownData ? (
				<p className="py-12 text-center text-sm text-content-secondary">
					{emptyMessage}
				</p>
			) : (
				<>
					{/* Models */}
					{sortedModels.length > 0 && (
						<div>
							<p className="m-0 mb-3 text-sm font-medium text-content-primary">
								Models
							</p>
							<div className="flex flex-col items-center gap-5 md:flex-row">
								<ChartContainer
									config={chartConfig}
									className="aspect-square h-44 w-44 shrink-0"
								>
									<PieChart>
										<ChartTooltip
											cursor={false}
											content={
												<ChartTooltipContent
													formatter={(value) => {
														const d = Number(value);
														return <span>${d.toFixed(2)}</span>;
													}}
													hideLabel={false}
												/>
											}
										/>
										<Pie
											data={pieData}
											dataKey="value"
											nameKey="name"
											cx="50%"
											cy="50%"
											innerRadius="60%"
											outerRadius="90%"
											paddingAngle={2}
											strokeWidth={0}
										>
											{pieData.map((_entry, i) => (
												<Cell
													key={sortedModels[i].model_config_id}
													fill={MODEL_COLORS[i % MODEL_COLORS.length]}
												/>
											))}
										</Pie>
									</PieChart>
								</ChartContainer>
								<div className="flex-1 space-y-0.5">
									{sortedModels.map((model, i) => {
										const pct =
											totalModelCost > 0
												? (model.total_cost_micros / totalModelCost) * 100
												: 0;
										return (
											<div
												key={model.model_config_id}
												className="flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 hover:bg-surface-secondary"
											>
												<span
													className="h-2.5 w-2.5 shrink-0 rounded-full"
													style={{
														backgroundColor:
															MODEL_COLORS[i % MODEL_COLORS.length],
													}}
												/>
												<span className="min-w-0 flex-1 truncate text-sm text-content-primary">
													{model.display_name || model.model}
													<span className="ml-1 text-content-secondary">
														{model.provider}
													</span>
												</span>
												<span
													className="rounded-full px-1.5 py-px text-xs font-medium"
													style={{
														backgroundColor:
															SURFACE_COLORS[i % SURFACE_COLORS.length],
														color: MODEL_COLORS[i % MODEL_COLORS.length],
													}}
												>
													{pct.toFixed(1)}%
												</span>
												<span className="w-16 text-right text-sm text-content-primary">
													{formatCostMicros(model.total_cost_micros)}
												</span>
											</div>
										);
									})}
								</div>
							</div>
						</div>
					)}

					{/* Top chats */}
					{topChats.length > 0 && (
						<div>
							<p className="m-0 mb-3 text-sm font-medium text-content-primary">
								Top chats
							</p>
							<div className="divide-y divide-border-default rounded-xl border border-border-default">
								{topChats.map((chat, i) => {
									const barPct =
										maxChatCost > 0
											? (chat.total_cost_micros / maxChatCost) * 100
											: 0;
									return (
										<div
											key={chat.root_chat_id}
											className="group relative flex items-center gap-3 px-3 py-2.5"
										>
											<div
												className="pointer-events-none absolute inset-y-0 left-0 bg-surface-purple opacity-[0.07] transition-opacity group-hover:opacity-[0.13]"
												style={{ width: `${barPct}%` }}
											/>
											<span className="relative z-10 w-5 shrink-0 text-right text-xs text-content-secondary">
												{i + 1}
											</span>
											<Link
												to={`/agents/${chat.root_chat_id}`}
												className="relative z-10 min-w-0 flex-1 truncate text-sm text-content-primary no-underline hover:underline"
											>
												{chat.chat_title || (
													<span className="italic text-content-secondary">
														Untitled
													</span>
												)}
											</Link>
											<span className="relative z-10 shrink-0 text-sm text-content-primary">
												{formatCostMicros(chat.total_cost_micros)}
											</span>
										</div>
									);
								})}
							</div>
						</div>
					)}
				</>
			)}
		</div>
	);
};
