import dayjs from "dayjs";
import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { formatTokenCount } from "utils/analytics";
import { formatCostMicros } from "utils/currency";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";

interface ChatCostSummaryViewProps {
	summary: TypesGen.ChatCostSummary | undefined;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
	loadingLabel: string;
	emptyMessage: string;
}

export const getUsageLimitPeriodLabel = (
	period: TypesGen.ChatUsageLimitPeriod | undefined,
): string => {
	if (!period) {
		return "";
	}

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
				className="flex min-h-[240px] items-center justify-center"
			>
				<Spinner size="lg" loading />
			</div>
		);
	}

	if (error != null) {
		return (
			<div className="flex min-h-[240px] flex-col items-center justify-center gap-4 text-center">
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

	const usageLimit = summary.usage_limit;
	const showUsageLimitCard = usageLimit?.is_limited === true;
	const usageLimitCurrentSpend = usageLimit?.current_spend ?? 0;
	const usageLimitSpendMicros = usageLimit?.spend_limit_micros ?? 0;
	const usageLimitPeriodLabel = usageLimit
		? getUsageLimitPeriodLabel(usageLimit.period)
		: "";
	const usageProgressPercentage =
		showUsageLimitCard && usageLimitSpendMicros > 0
			? Math.min((usageLimitCurrentSpend / usageLimitSpendMicros) * 100, 100)
			: 0;
	const usageProgressBarClass =
		usageProgressPercentage > 90
			? "bg-content-destructive"
			: usageProgressPercentage >= 75
				? "bg-content-warning"
				: "bg-content-success";
	const usageLimitExceeded =
		showUsageLimitCard && usageLimitCurrentSpend >= usageLimitSpendMicros;
	const usageLimitStatusText = usageLimitExceeded
		? "Limit exceeded"
		: `${formatCostMicros(
				Math.max(usageLimitSpendMicros - usageLimitCurrentSpend, 0),
			)} remaining`;
	const usageLimitCurrentPeriod =
		showUsageLimitCard && usageLimit?.period_start && usageLimit?.period_end
			? `Current period: ${dayjs(usageLimit.period_start).format("MMM D")} – ${dayjs(
					usageLimit.period_end,
				).format("MMM D")}`
			: "";
	const usageLimitResetAt =
		showUsageLimitCard && usageLimit?.period_end
			? dayjs(usageLimit.period_end).format("MMM D, YYYY h:mm A")
			: "";

	return (
		<div className="space-y-6">
			<div className="grid grid-cols-2 gap-4 md:grid-cols-3">
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Total Cost
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{formatCostMicros(summary.total_cost_micros)}
					</p>
				</div>
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Input Tokens
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{formatTokenCount(summary.total_input_tokens)}
					</p>
				</div>
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Output Tokens
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{formatTokenCount(summary.total_output_tokens)}
					</p>
				</div>
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Cache Read
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{formatTokenCount(summary.total_cache_read_tokens)}
					</p>
				</div>
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Cache Write
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{formatTokenCount(summary.total_cache_creation_tokens)}
					</p>
				</div>
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Messages
					</p>
					<p className="mt-1 text-2xl font-semibold text-content-primary">
						{(
							summary.priced_message_count + summary.unpriced_message_count
						).toLocaleString()}
					</p>
				</div>
			</div>

			{showUsageLimitCard && usageLimit && (
				<div className="rounded-lg border border-border-default bg-surface-secondary p-4">
					<div className="flex flex-col gap-4">
						<div className="flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
							<div>
								<p className="text-xs font-medium uppercase tracking-wide text-content-secondary">
									{usageLimitPeriodLabel} Spend Limit
								</p>
								{usageLimitCurrentPeriod && (
									<p className="mt-1 text-sm text-content-secondary">
										{usageLimitCurrentPeriod}
									</p>
								)}
								<p className="mt-1 text-2xl font-semibold text-content-primary">
									{formatCostMicros(usageLimitCurrentSpend)} /{" "}
									{formatCostMicros(usageLimitSpendMicros)}
								</p>
							</div>
							<p className="text-sm text-content-secondary">
								{Math.round(usageProgressPercentage)}% used
							</p>
						</div>
						<div
							role="progressbar"
							aria-label={`${usageLimitPeriodLabel} spend usage`}
							aria-valuemin={0}
							aria-valuemax={100}
							aria-valuenow={Math.round(usageProgressPercentage)}
							className="h-2 overflow-hidden rounded-full bg-surface-tertiary"
						>
							<div
								className={`h-full rounded-full ${usageProgressBarClass}`}
								style={{ width: `${usageProgressPercentage}%` }}
							/>
						</div>
						<div className="flex flex-col gap-1 text-sm md:flex-row md:items-center md:justify-between">
							<p
								className={
									usageLimitExceeded
										? "text-content-destructive"
										: "text-content-secondary"
								}
							>
								{usageLimitStatusText}
							</p>
							<p className="text-content-secondary">
								Resets {usageLimitResetAt}
							</p>
						</div>
					</div>
				</div>
			)}

			{summary.unpriced_message_count > 0 && (
				<div className="flex items-start gap-3 rounded-lg border border-border-warning bg-surface-warning p-4 text-sm text-content-primary">
					<TriangleAlertIcon className="h-5 w-5 shrink-0 text-content-warning" />
					<span>
						{summary.unpriced_message_count} message
						{summary.unpriced_message_count === 1 ? "" : "s"} could not be
						priced because model pricing data was unavailable.
					</span>
				</div>
			)}

			{summary.by_model.length === 0 && summary.by_chat.length === 0 ? (
				<p className="py-12 text-center text-content-secondary">
					{emptyMessage}
				</p>
			) : (
				<>
					<div className="overflow-x-auto rounded-lg border border-border-default">
						<Table className="text-sm" aria-label="Cost breakdown by model">
							<TableHeader>
								<TableRow className="text-left text-xs font-medium uppercase tracking-wide text-content-secondary">
									<TableHead className="px-4 py-3">Model</TableHead>
									<TableHead className="px-4 py-3">Provider</TableHead>
									<TableHead className="px-4 py-3 text-right">Cost</TableHead>
									<TableHead className="px-4 py-3 text-right">
										Messages
									</TableHead>
									<TableHead className="px-4 py-3 text-right">Input</TableHead>
									<TableHead className="px-4 py-3 text-right">Output</TableHead>
									<TableHead className="px-4 py-3 text-right">
										Cache Read
									</TableHead>
									<TableHead className="px-4 py-3 text-right">
										Cache Write
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{summary.by_model.map((model) => (
									<TableRow
										key={model.model_config_id}
										className="border-t border-border-default"
									>
										<TableCell className="px-4 py-3">
											{model.display_name || model.model}
										</TableCell>
										<TableCell className="px-4 py-3 text-content-secondary">
											{model.provider}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatCostMicros(model.total_cost_micros)}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{model.message_count.toLocaleString()}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatTokenCount(model.total_input_tokens)}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatTokenCount(model.total_output_tokens)}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatTokenCount(model.total_cache_read_tokens)}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatTokenCount(model.total_cache_creation_tokens)}
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					</div>

					<div className="overflow-x-auto rounded-lg border border-border-default">
						<Table className="text-sm" aria-label="Cost breakdown by chat">
							<TableHeader>
								<TableRow className="text-left text-xs font-medium uppercase tracking-wide text-content-secondary">
									<TableHead className="px-4 py-3">Chat</TableHead>
									<TableHead className="px-4 py-3 text-right">Cost</TableHead>
									<TableHead className="px-4 py-3 text-right">
										Messages
									</TableHead>
									<TableHead className="px-4 py-3 text-right">Input</TableHead>
									<TableHead className="px-4 py-3 text-right">Output</TableHead>
									<TableHead className="px-4 py-3 text-right">
										Cache Read
									</TableHead>
									<TableHead className="px-4 py-3 text-right">
										Cache Write
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{summary.by_chat.map((chat) => (
									<TableRow
										key={chat.root_chat_id}
										className="border-t border-border-default"
									>
										<TableCell className="px-4 py-3">
											{chat.chat_title || (
												<span className="italic text-content-secondary">
													Untitled chat
												</span>
											)}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatCostMicros(chat.total_cost_micros)}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{chat.message_count.toLocaleString()}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatTokenCount(chat.total_input_tokens)}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatTokenCount(chat.total_output_tokens)}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatTokenCount(chat.total_cache_read_tokens)}
										</TableCell>
										<TableCell className="px-4 py-3 text-right">
											{formatTokenCount(chat.total_cache_creation_tokens)}
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					</div>
				</>
			)}
		</div>
	);
};
