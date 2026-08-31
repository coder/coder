import dayjs from "dayjs";
import { BanIcon, InfoIcon } from "lucide-react";
import type { FC } from "react";
import type { Feature } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
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

	const {
		limit,
		soft_limit: softLimit,
		hard_limit: hardLimit,
		actual_ms: actualMs,
		usage_period: usagePeriod,
	} = feature;

	// An omitted limit means the license grants unlimited runtime hours.
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
	const usedTenths =
		actualMs === undefined ? 0 : Math.floor(actualMs / 360_000);
	const usedHours = usedTenths / 10;
	const hardCap =
		!isUnlimited &&
		hardLimit !== undefined &&
		hardLimit > 0 &&
		hardLimit >= meteredLimit
			? hardLimit
			: undefined;
	const reachedAllocation =
		!isUnlimited && actualMs !== undefined && usedHours >= meteredLimit;
	const reachedHardCap =
		hardCap !== undefined && actualMs !== undefined && usedHours >= hardCap;
	const reachedSoftLimit =
		!isUnlimited &&
		!reachedAllocation &&
		actualMs !== undefined &&
		softLimit !== undefined &&
		usedHours >= softLimit;
	const barScale = hardCap ?? meteredLimit;
	const usagePercentage = isUnlimited
		? 100
		: barScale > 0
			? Math.min((usedHours / barScale) * 100, 100)
			: 0;
	const allocationMarkerPercent =
		hardCap === undefined
			? undefined
			: Math.min((meteredLimit / hardCap) * 100, 100);

	const softMarkerPercent =
		!isUnlimited && softLimit !== undefined && barScale > 0
			? Math.min((softLimit / barScale) * 100, 100)
			: undefined;
	const fillClassName = reachedAllocation
		? "bg-highlight-red"
		: reachedSoftLimit
			? "bg-border-warning"
			: "bg-highlight-green";

	// Already floored to tenths, so rendering one decimal never rounds.
	const usedLabel =
		actualMs === undefined
			? "N/A"
			: usedHours.toLocaleString("en-US", {
					minimumFractionDigits: 1,
					maximumFractionDigits: 1,
				});
	const warningLabel =
		!isUnlimited && softLimit !== undefined
			? softLimit.toLocaleString("en-US")
			: undefined;
	const allocationLabel = isUnlimited
		? "Unlimited"
		: meteredLimit.toLocaleString("en-US");
	const limitLabel = hardCap?.toLocaleString("en-US");

	const periodStart = usagePeriod ? dayjs(usagePeriod.start) : undefined;
	const periodEnd = usagePeriod ? dayjs(usagePeriod.end) : undefined;
	const usagePeriodLabel =
		periodStart?.isValid() && periodEnd?.isValid()
			? `(${periodStart.format("MMMM D, YYYY")} - ${periodEnd.format("MMMM D, YYYY")})`
			: undefined;

	const formatPercent = (numerator: number, denominator: number): string =>
		(Math.floor((numerator * 1000) / denominator) / 10).toLocaleString("en-US");

	const softLimitPercent =
		!isUnlimited && softLimit !== undefined && meteredLimit > 0
			? formatPercent(softLimit, meteredLimit)
			: undefined;

	// A fading green fill reads as an unmetered allocation rather than
	// 100% usage. The mask needs the -webkit- prefix for Safari.
	const unlimitedBarClassName = cn(
		"bg-highlight-green",
		"mask-[linear-gradient(to_right,black_50%,transparent_100%)]",
		"[-webkit-mask-image:linear-gradient(to_right,black_50%,transparent_100%)]",
	);

	let tooltip: string;
	if (reachedAllocation) {
		const usedPercent =
			meteredLimit > 0 ? formatPercent(usedTenths, meteredLimit * 10) : "100";
		tooltip = reachedHardCap
			? `You've used ${usedPercent}% of your Total Agent hours for this license and reached the hard cap of ${limitLabel} hours. Contact sales to receive more Agent hours.`
			: `You've used ${usedPercent}% of your Total Agent hours for this license. Contact sales to receive more Agent hours.`;
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
					<div className="flex flex-col gap-0.5">
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
						{usagePeriodLabel && (
							<p className="m-0 text-xs text-content-secondary">
								{usagePeriodLabel}
							</p>
						)}
					</div>

					{reachedHardCap && (
						<div className="flex items-center justify-end">
							{/* The concurrency cap re-engages at usage equal to the
							    hard cap, so the badge shows at the inclusive boundary
							    and reads "reached" (still true above it) rather than
							    "exceeded" (false at equality). The sentence outgrows
							    narrow cards, so this instance wraps instead of
							    keeping the shared Badge's single-line layout. */}
							<Badge
								variant="destructive"
								size="sm"
								className="h-auto rounded-full text-wrap"
							>
								<BanIcon />
								Agent hours limit reached. Concurrent chats are now limited to
								5.
							</Badge>
						</div>
					)}

					<div
						className="relative h-5 w-full overflow-hidden bg-surface-secondary"
						aria-hidden="true"
					>
						{!isUnlimited && (
							<>
								{softMarkerPercent !== undefined && (
									<div
										className="absolute inset-y-0 z-0 border-0 border-l-2 border-dotted border-border-warning"
										style={{ left: `${softMarkerPercent}%` }}
									/>
								)}
								{hardCap === undefined ? (
									// The allocation marker sits at the track's right edge.
									<div className="absolute inset-y-0 right-0 z-0 w-0.5 bg-highlight-red" />
								) : (
									<div
										className="absolute inset-y-0 z-0 w-0.5 bg-highlight-red"
										style={{ left: `${allocationMarkerPercent}%` }}
									/>
								)}
							</>
						)}
						{isUnlimited ? (
							<div
								className={cn(
									"relative z-10 h-full w-full",
									unlimitedBarClassName,
								)}
							/>
						) : (
							<div
								className={cn(
									"relative z-10 h-full transition-[width] duration-300",
									fillClassName,
								)}
								style={{ width: `${usagePercentage}%` }}
							/>
						)}
					</div>

					<div className="flex items-start justify-between gap-3 text-sm font-medium">
						<p className="m-0 whitespace-nowrap text-content-primary">
							<span className="text-content-secondary">Used: </span>
							<span
								className={cn({
									"text-content-destructive": reachedAllocation,
									"text-border-warning": reachedSoftLimit,
								})}
							>
								{usedLabel}
							</span>
						</p>
						<div className="flex flex-wrap items-start justify-end gap-x-3 gap-y-1 whitespace-nowrap text-content-secondary">
							{warningLabel !== undefined && (
								<p className="m-0">
									Warning:{" "}
									<span className="text-content-primary">{warningLabel}</span>
								</p>
							)}
							<p className="m-0">
								Allocation:{" "}
								<span className="text-content-primary">{allocationLabel}</span>
							</p>
							{limitLabel !== undefined && (
								<p className="m-0">
									Limit:{" "}
									<span className="text-content-primary">{limitLabel}</span>
								</p>
							)}
						</div>
					</div>
				</div>
			</div>
		</section>
	);
};
