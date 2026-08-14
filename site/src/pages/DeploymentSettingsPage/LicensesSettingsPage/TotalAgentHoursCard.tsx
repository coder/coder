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
	// Floored to tenths so the displayed number and the reached states
	// below flip together at the backend's whole-hour thresholds.
	const usedHours =
		actualMs === undefined ? 0 : Math.floor(actualMs / 360_000) / 10;
	// The backend guarantees hard >= allocation > 0; anything else comes
	// from a decoding bug and is ignored the same way.
	const hardCap =
		!isUnlimited &&
		hardLimit !== undefined &&
		hardLimit > 0 &&
		hardLimit >= meteredLimit
			? hardLimit
			: undefined;
	// The backend warns with >=, so reaching a threshold (not exceeding
	// it) drives the warning copy and colors.
	const reachedAllocation =
		!isUnlimited && actualMs !== undefined && usedHours >= meteredLimit;
	const reachedHardCap =
		hardCap !== undefined && actualMs !== undefined && usedHours >= hardCap;
	// Missing usage data must not count as reaching a zero soft limit,
	// which is valid and reached by any known usage.
	const reachedSoftLimit =
		!isUnlimited &&
		!reachedAllocation &&
		actualMs !== undefined &&
		softLimit !== undefined &&
		usedHours >= softLimit;
	// With a hard cap the track spans the hard-cap range and the
	// allocation falls at an interior marker.
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
	// Near the right edge the limit label would collide with the hard cap
	// label, so the hard cap text moves above the bar.
	const hardCapLabelAboveBar =
		allocationMarkerPercent !== undefined && allocationMarkerPercent > 85;
	// Near the left edge the centered limit label would spill outside the
	// card, so it moves above the bar instead.
	const limitLabelAboveBar =
		allocationMarkerPercent !== undefined && allocationMarkerPercent < 15;
	// On narrow cards an interior-marker limit label can collide with the
	// hard cap label, so below the md breakpoint it moves above the bar.
	const limitLabelStacksNarrow =
		allocationMarkerPercent !== undefined &&
		!limitLabelAboveBar &&
		!hardCapLabelAboveBar;

	// The fill is segmented by position: green to the soft limit, yellow
	// to the allocation, and red beyond it.
	const softMarkerPercent =
		!isUnlimited && softLimit !== undefined && barScale > 0
			? Math.min((softLimit / barScale) * 100, 100)
			: undefined;
	const limitBoundaryPercent = allocationMarkerPercent ?? 100;
	// An allocation at the track's right edge leaves no room for a red
	// segment past it, so reaching it turns the whole fill red instead.
	const fullRedFill = reachedAllocation && limitBoundaryPercent >= 100;
	const greenWidth = fullRedFill
		? 0
		: Math.min(usagePercentage, softMarkerPercent ?? limitBoundaryPercent);
	const yellowWidth =
		fullRedFill || softMarkerPercent === undefined
			? 0
			: Math.max(
					0,
					Math.min(usagePercentage, limitBoundaryPercent) - softMarkerPercent,
				);
	const redLeft = fullRedFill ? 0 : limitBoundaryPercent;
	const redWidth = fullRedFill
		? usagePercentage
		: Math.max(0, usagePercentage - limitBoundaryPercent);

	// Already floored to tenths, so rendering one decimal never rounds.
	const usedLabel =
		actualMs === undefined
			? "N/A"
			: usedHours.toLocaleString("en-US", {
					minimumFractionDigits: 1,
					maximumFractionDigits: 1,
				});
	const limitLabel = isUnlimited
		? "Unlimited"
		: meteredLimit.toLocaleString("en-US");
	const hardCapLabel = hardCap?.toLocaleString("en-US");

	// The license period the usage covers. A missing or unparsable period
	// omits the dates rather than replacing meaningful usage with an error.
	const periodStart = usagePeriod ? dayjs(usagePeriod.start) : undefined;
	const periodEnd = usagePeriod ? dayjs(usagePeriod.end) : undefined;
	const usagePeriodLabels =
		periodStart?.isValid() && periodEnd?.isValid()
			? {
					start: periodStart.format("MMMM D, YYYY"),
					end: periodEnd.format("MMMM D, YYYY"),
				}
			: undefined;

	// Floored at one decimal so the shown percentage never crosses a
	// threshold the underlying hours have not (99.9%, never a false 100%).
	const formatPercent = (value: number): string =>
		(Math.floor(value * 10) / 10).toLocaleString("en-US");

	const softLimitPercent =
		!isUnlimited && softLimit !== undefined && meteredLimit > 0
			? formatPercent((softLimit / meteredLimit) * 100)
			: undefined;

	// Fading green stripes read as an unmetered allocation rather than
	// 100% usage. The mask needs the -webkit- prefix for Safari.
	const unlimitedBarClassName = cn(
		"bg-[repeating-linear-gradient(-45deg,hsl(var(--highlight-green)),hsl(var(--highlight-green))_6px,transparent_6px,transparent_12px)]",
		"[mask-image:linear-gradient(to_right,black_50%,transparent_100%)]",
		"[-webkit-mask-image:linear-gradient(to_right,black_50%,transparent_100%)]",
	);

	let tooltip: string;
	if (reachedAllocation) {
		// Measured against the allocation even when a hard cap widens the
		// bar. The zero-limit fallback is unreachable for well-formed
		// licenses, which report zero-hour allocations as disabled.
		const usedPercent =
			meteredLimit > 0
				? formatPercent((usedHours / meteredLimit) * 100)
				: "100";
		tooltip = reachedHardCap
			? `You've used ${usedPercent}% of your Total Agent hours for this license and reached the hard cap of ${hardCapLabel} hours. Contact sales to receive more Agent hours.`
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

					{usagePeriodLabels && (
						<div className="flex items-center justify-between text-sm text-content-secondary">
							<span>{usagePeriodLabels.start}</span>
							<span>{usagePeriodLabels.end}</span>
						</div>
					)}

					{(reachedHardCap ||
						hardCapLabelAboveBar ||
						limitLabelAboveBar ||
						limitLabelStacksNarrow) && (
						<div
							className={cn(
								"flex items-center justify-end gap-3",
								!reachedHardCap &&
									!hardCapLabelAboveBar &&
									!limitLabelAboveBar &&
									"md:hidden",
							)}
						>
							{(limitLabelAboveBar || limitLabelStacksNarrow) && (
								// Offset to the marker by a percentage margin (the row
								// and the track share a width); the narrow-only variant
								// left-aligns instead.
								<p
									className={cn(
										"m-0 mr-auto text-sm font-medium whitespace-nowrap text-content-secondary",
										limitLabelStacksNarrow && "md:hidden",
									)}
									style={
										limitLabelAboveBar
											? { marginLeft: `${allocationMarkerPercent}%` }
											: undefined
									}
								>
									Limit:{" "}
									<span className="text-content-primary">{limitLabel}</span>
								</p>
							)}
							{reachedHardCap && (
								<Badge variant="destructive" size="sm" className="rounded-full">
									<BanIcon />
									Hard cap reached
								</Badge>
							)}
							{hardCapLabelAboveBar && (
								<p className="m-0 text-sm font-medium whitespace-nowrap text-content-secondary">
									Hard cap:{" "}
									<span className="text-content-primary">{hardCapLabel}</span>
								</p>
							)}
						</div>
					)}

					{/* The marker lines overshoot the track on both sides, so they
					    live outside the track's overflow clipping. */}
					<div className="relative" aria-hidden="true">
						<div className="relative h-5 w-full overflow-hidden rounded bg-surface-secondary">
							{isUnlimited ? (
								<div
									className={cn(
										"h-full w-full rounded-l",
										unlimitedBarClassName,
									)}
								/>
							) : (
								<>
									<div
										className="absolute inset-y-0 left-0 rounded-l bg-highlight-green transition-[width] duration-300"
										style={{ width: `${greenWidth}%` }}
									/>
									{softMarkerPercent !== undefined && (
										<div
											className="absolute inset-y-0 bg-yellow-400 transition-[width] duration-300"
											style={{
												left: `${softMarkerPercent}%`,
												width: `${yellowWidth}%`,
											}}
										/>
									)}
									{(hardCap !== undefined || fullRedFill) && (
										<div
											className={cn(
												"absolute inset-y-0 bg-highlight-red transition-[width] duration-300",
												fullRedFill && "rounded-l",
											)}
											style={{
												left: `${redLeft}%`,
												width: `${redWidth}%`,
											}}
										/>
									)}
								</>
							)}
						</div>
						{!isUnlimited && (
							<>
								{softMarkerPercent !== undefined && (
									// Palette yellow: the theme has no yellow highlight token.
									<div
										className="absolute -inset-y-1 border-0 border-l-2 border-dotted border-yellow-400"
										style={{ left: `${softMarkerPercent}%` }}
									/>
								)}
								{hardCap === undefined ? (
									// The allocation marker sits at the track's right edge.
									<div className="absolute -inset-y-1 right-0 w-0.5 bg-highlight-red" />
								) : (
									<>
										<div
											className="absolute -inset-y-1 w-0.5 bg-highlight-red"
											style={{ left: `${allocationMarkerPercent}%` }}
										/>
										{/* The primary content color stays visible over both
										    themes' card backgrounds. */}
										<div className="absolute -inset-y-1 right-0 w-1 bg-content-primary" />
									</>
								)}
							</>
						)}
					</div>

					<div className="relative flex items-start justify-between text-sm font-medium whitespace-nowrap">
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
						{allocationMarkerPercent === undefined ? (
							<p className="m-0 text-content-secondary">
								Limit:{" "}
								<span className="text-content-primary">{limitLabel}</span>
							</p>
						) : (
							<>
								{/* The limit label follows its marker: right-aligned
								    against it near the right edge, otherwise centered
								    (with the narrow copy rendering above the bar). */}
								{!limitLabelAboveBar && (
									<p
										className={cn(
											"absolute m-0 text-content-secondary",
											hardCapLabelAboveBar
												? "-translate-x-full"
												: "hidden -translate-x-1/2 md:block",
										)}
										style={{ left: `${allocationMarkerPercent}%` }}
									>
										Limit:{" "}
										<span className="text-content-primary">{limitLabel}</span>
									</p>
								)}
								{!hardCapLabelAboveBar && (
									<p className="m-0 text-content-secondary">
										Hard cap:{" "}
										<span className="text-content-primary">{hardCapLabel}</span>
									</p>
								)}
							</>
						)}
					</div>
				</div>
			</div>
		</section>
	);
};
