import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import type { Feature } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

type TotalAgentHoursCardProps = {
	feature?: Feature;
};

export const TotalAgentHoursCard: FC<TotalAgentHoursCardProps> = ({
	feature,
}) => {
	// A zero-hour allocation arrives with enabled=false, which hides the
	// panel entirely rather than showing an empty bar.
	if (!feature?.enabled) {
		return null;
	}

	const { limit, soft_limit: softLimit, actual } = feature;

	// An enabled feature with the limit omitted is the unlimited
	// allocation: the license grants unlimited runtime hours and carries no
	// thresholds, so the bar renders full and neutral like
	// SeatUsageBarCard's allowUnlimited state.
	const isUnlimited = limit === undefined;

	if (!isUnlimited && limit < 0) {
		return (
			<section className="border border-solid rounded">
				<div className="p-4">
					<ErrorAlert error="Invalid license usage limits" />
				</div>
			</section>
		);
	}

	const meteredLimit = limit ?? 0;
	const usedHours = actual ?? 0;
	// The backend warns with >= for both thresholds, so "reached" (not
	// "exceeded") flips the bar color. An unlimited allocation has no
	// thresholds to reach.
	const reachedAllocation =
		!isUnlimited && actual !== undefined && usedHours >= meteredLimit;
	// Missing usage data (the usage query failed) must not count as
	// reaching the soft limit: a soft limit of zero is valid and would
	// otherwise compare as reached against the defaulted zero usage.
	const reachedSoftLimit =
		!isUnlimited &&
		!reachedAllocation &&
		actual !== undefined &&
		softLimit !== undefined &&
		usedHours >= softLimit;
	const usagePercentage = isUnlimited
		? 100
		: meteredLimit > 0
			? Math.min((usedHours / meteredLimit) * 100, 100)
			: 0;

	const usedLabel =
		actual === undefined ? "\u2014" : usedHours.toLocaleString("en-US");
	const limitLabel = isUnlimited
		? "Unlimited"
		: meteredLimit.toLocaleString("en-US");

	// Floored so the percentage never crosses a boundary the underlying
	// hours have not reached: a soft limit of 999/1,000 hours displays as
	// 99%, never rounding up to a false 100%.
	const softLimitPercent =
		!isUnlimited && softLimit !== undefined && meteredLimit > 0
			? Math.floor((softLimit / meteredLimit) * 100)
			: undefined;

	let tooltip: string;
	if (reachedAllocation) {
		// Flooring cannot understate reaching the allocation because usage
		// is at least the limit here, so the percentage is at least 100 and
		// reports over-allocation (1,200 of 1,000 hours shows 120%). The
		// zero-limit fallback is unreachable for well-formed licenses: the
		// backend reports a zero-hour allocation as a disabled feature,
		// which hides the card entirely.
		const usedPercent =
			meteredLimit > 0 ? Math.floor((usedHours / meteredLimit) * 100) : 100;
		tooltip = `You've used ${usedPercent}% of your Total Agent hours for this license. Contact sales to receive more Agent hours.`;
	} else if (reachedSoftLimit) {
		tooltip = `You've used ${softLimitPercent}% or more of your Total Agent hours for this license. Agent sessions are still working normally, but you'll want to plan for the 100% limit.`;
	} else if (softLimitPercent !== undefined) {
		tooltip = `Total time agents have been working across all workspaces this license. A soft-limit warning appears at ${softLimitPercent}%`;
	} else {
		tooltip =
			"Total time agents have been working across all workspaces this license.";
	}

	return (
		<section className="border border-solid rounded">
			<div className="p-4">
				<div className="flex flex-col gap-2">
					<div className="flex items-center gap-1">
						<h3 className="text-md m-0 font-medium">Total agent hours</h3>
						<Tooltip>
							<TooltipTrigger asChild>
								<button
									type="button"
									aria-label="Total agent hours information"
									className="m-0 inline-flex appearance-none border-0 bg-transparent p-0 text-content-secondary"
								>
									<InfoIcon className="size-3" />
								</button>
							</TooltipTrigger>
							<TooltipContent side="top" className="max-w-xs">
								{tooltip}
							</TooltipContent>
						</Tooltip>
					</div>

					<div
						className="relative h-5 w-full overflow-hidden rounded bg-surface-secondary"
						aria-hidden="true"
					>
						<div
							className={cn(
								"h-full rounded-l transition-[width] duration-300",
								reachedAllocation
									? "bg-highlight-red"
									: reachedSoftLimit
										? "bg-highlight-orange"
										: "bg-highlight-green",
							)}
							style={{ width: `${usagePercentage}%` }}
						/>
					</div>

					<div className="flex items-start justify-between text-sm font-medium whitespace-nowrap">
						<p className="m-0 text-content-primary">
							<span className="text-content-secondary">Used: </span>
							<span
								className={cn({
									"text-content-destructive": reachedAllocation,
								})}
							>
								{usedLabel}
							</span>
						</p>
						<p className="m-0 text-content-secondary">
							Limit: <span className="text-content-primary">{limitLabel}</span>
						</p>
					</div>
				</div>
			</div>
		</section>
	);
};
