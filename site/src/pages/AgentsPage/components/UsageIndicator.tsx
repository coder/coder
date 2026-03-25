import dayjs from "dayjs";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link } from "react-router";
import { formatCostMicros } from "utils/currency";
import { chatUsageLimitStatus } from "#/api/queries/chats";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { getUsageLimitPeriodLabel } from "./ChatCostSummaryView";

export const UsageIndicator: FC = () => {
	const { data, isLoading, isError } = useQuery(chatUsageLimitStatus());

	if (isLoading || isError || !data?.is_limited) {
		return null;
	}

	const spendLimit = data.spend_limit_micros ?? 0;
	const currentSpend = data.current_spend;
	const percent =
		spendLimit > 0 ? Math.min((currentSpend / spendLimit) * 100, 100) : 0;
	const roundedPercent = Math.round(percent);
	const exceeded = spendLimit > 0 && currentSpend >= spendLimit;
	const periodLabel = getUsageLimitPeriodLabel(data.period);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="ml-auto flex self-stretch flex-col justify-center items-start gap-1 border-none bg-transparent px-3 cursor-pointer select-none transition-colors text-content-secondary hover:bg-surface-tertiary/50 outline-none text-[13px]"
				>
					<span className="shrink-0 whitespace-nowrap">
						{periodLabel} Usage
					</span>
					<div
						role="progressbar"
						aria-label={`${periodLabel} spend usage`}
						aria-valuemin={0}
						aria-valuemax={100}
						aria-valuenow={roundedPercent}
						className="h-1.5 w-full overflow-hidden rounded-full bg-surface-tertiary shrink-0"
					>
						<div
							className="h-full rounded-full bg-content-secondary transition-all duration-300 ease-out"
							style={{ width: `${percent}%` }}
						/>
					</div>
				</button>
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end" className="min-w-auto w-[240px]">
				{/* Header */}
				<div className="flex items-center justify-between px-2 py-1.5">
					<span className="text-sm font-medium text-content-primary">
						{periodLabel} Usage
					</span>
					<span className="text-xs text-content-secondary">
						{roundedPercent}%
					</span>
				</div>

				{/* Progress bar */}
				<div className="px-2 pb-2">
					<div
						role="progressbar"
						aria-label={`${periodLabel} spend usage`}
						aria-valuemin={0}
						aria-valuemax={100}
						aria-valuenow={roundedPercent}
						className="h-1.5 overflow-hidden rounded-full bg-surface-tertiary"
					>
						<div
							className="h-full rounded-full bg-content-secondary transition-all duration-300 ease-out"
							style={{ width: `${percent}%` }}
						/>
					</div>
				</div>

				{/* Spend detail */}
				<div className="px-2 pb-1.5 text-xs text-content-secondary">
					{formatCostMicros(currentSpend)} of {formatCostMicros(spendLimit)}{" "}
					used
					{exceeded && (
						<span className="ml-1 text-content-destructive">
							— limit exceeded
						</span>
					)}
				</div>

				{data.period_end && (
					<div className="px-2 pb-2 text-xs text-content-secondary">
						Resets {dayjs(data.period_end).format("MMM D, YYYY")}
					</div>
				)}

				<DropdownMenuSeparator />

				<DropdownMenuItem asChild>
					<Link to="/agents/analytics">View usage</Link>
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
