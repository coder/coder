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
	} = feature;

	// An enabled feature with the limit omitted is the unlimited
	// allocation: the license grants unlimited runtime hours and carries
	// no thresholds to warn about.
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
	// Usage in tenths of hours, floored via integer math from the exact
	// milliseconds so the displayed number and the reached states below
	// flip at the same instant as the backend's whole-hour thresholds
	// (floor_tenths(x) >= N is equivalent to x >= N for integer N).
	const usedHours =
		actualMs === undefined ? 0 : Math.floor(actualMs / 360_000) / 10;
	// The backend only attaches a hard limit to a positive allocation and
	// guarantees hard >= allocation, ignoring unusable hard limit claims,
	// so anything else here comes from a decoding bug and is ignored the
	// same way.
	const hardCap =
		!isUnlimited &&
		hardLimit !== undefined &&
		hardLimit > 0 &&
		hardLimit >= meteredLimit
			? hardLimit
			: undefined;
	// The backend warns with >= for both thresholds, so "reached" (not
	// "exceeded") drives the warning copy and the used-label color. An
	// unlimited allocation has no thresholds to reach.
	const reachedAllocation =
		!isUnlimited && actualMs !== undefined && usedHours >= meteredLimit;
	const reachedHardCap =
		hardCap !== undefined && actualMs !== undefined && usedHours >= hardCap;
	// Missing usage data (the usage query failed) must not count as
	// reaching the soft limit: a soft limit of zero is valid and would
	// otherwise compare as reached against the defaulted zero usage.
	const reachedSoftLimit =
		!isUnlimited &&
		!reachedAllocation &&
		actualMs !== undefined &&
		softLimit !== undefined &&
		usedHours >= softLimit;
	// With a hard cap the track spans the full enforcement range: its
	// right edge is the hard cap and the allocation falls at an interior
	// marker.
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
	// When the allocation marker lands near the track's right edge, the
	// limit label under the marker would collide with the hard cap label,
	// so the hard cap text moves above the bar.
	const hardCapLabelAboveBar =
		allocationMarkerPercent !== undefined && allocationMarkerPercent > 85;
	// Near the left edge the centered limit label would spill outside the
	// card and over the Used label, so the limit text moves above the bar
	// instead, left-aligned at its marker.
	const limitLabelAboveBar =
		allocationMarkerPercent !== undefined && allocationMarkerPercent < 15;

	// The fill is segmented by position instead of switching color as a
	// whole: green until the soft limit, yellow from the soft limit to the
	// allocation, and red from the allocation to the hard cap. Each
	// threshold also carries a marker line on the track at the same
	// position, so a marker only stands out against fill of a different
	// color once usage passes it.
	const softMarkerPercent =
		!isUnlimited && softLimit !== undefined && barScale > 0
			? Math.min((softLimit / barScale) * 100, 100)
			: undefined;
	const limitBoundaryPercent = allocationMarkerPercent ?? 100;
	const greenWidth = Math.min(
		usagePercentage,
		softMarkerPercent ?? limitBoundaryPercent,
	);
	const yellowWidth =
		softMarkerPercent === undefined
			? 0
			: Math.max(
					0,
					Math.min(usagePercentage, limitBoundaryPercent) - softMarkerPercent,
				);
	const redWidth = Math.max(0, usagePercentage - limitBoundaryPercent);

	// Usage always renders with exactly one decimal (e.g. 42.0, 10.3). The
	// value is already floored to tenths, so no rounding happens here. The
	// limit and hard cap labels stay whole because the claims are whole
	// hours. Missing usage data falls back to N/A rather than a dash.
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

	// Percentages are floored at one decimal place so the shown value
	// never crosses a threshold the underlying hours have not reached: a
	// soft limit of 999/1,000 hours renders as 99.9%, never a false 100%.
	// Whole values render without a trailing decimal.
	const formatPercent = (value: number): string =>
		(Math.floor(value * 10) / 10).toLocaleString("en-US");

	const softLimitPercent =
		!isUnlimited && softLimit !== undefined && meteredLimit > 0
			? formatPercent((softLimit / meteredLimit) * 100)
			: undefined;

	// An unlimited allocation renders as green diagonal stripes whose
	// right half fades out into the track: the hatched, trailing-off bar
	// reads as an unmetered allocation rather than 100% usage. The mask
	// needs the -webkit- prefix for Safari.
	const unlimitedBarClassName = cn(
		"bg-[repeating-linear-gradient(-45deg,hsl(var(--highlight-green)),hsl(var(--highlight-green))_6px,transparent_6px,transparent_12px)]",
		"[mask-image:linear-gradient(to_right,black_50%,transparent_100%)]",
		"[-webkit-mask-image:linear-gradient(to_right,black_50%,transparent_100%)]",
	);

	let tooltip: string;
	if (reachedAllocation) {
		// The percentage is measured against the allocation even when a
		// hard cap widens the bar, and flooring cannot understate reaching
		// the allocation because usage is at least the limit here (1,200 of
		// 1,000 hours shows 120%). The zero-limit fallback is unreachable
		// for well-formed licenses: the backend reports a zero-hour
		// allocation as a disabled feature, which hides the card entirely.
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

					{(reachedHardCap || hardCapLabelAboveBar || limitLabelAboveBar) && (
						<div className="flex items-center justify-end gap-3">
							{limitLabelAboveBar && (
								// In-flow so the row keeps its height, offset to the
								// marker by a percentage margin (the row and the track
								// share a width), with the auto margin keeping the
								// enforcement pill on the right.
								<p
									className="m-0 mr-auto text-sm font-medium whitespace-nowrap text-content-secondary"
									style={{ marginLeft: `${allocationMarkerPercent}%` }}
								>
									Limit:{" "}
									<span className="text-content-primary">{limitLabel}</span>
								</p>
							)}
							{reachedHardCap && (
								<Badge variant="destructive" size="sm" className="rounded-full">
									<BanIcon />
									Hard cap reached - chat concurrency enforced
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
									{hardCap !== undefined && (
										<div
											className="absolute inset-y-0 bg-highlight-red transition-[width] duration-300"
											style={{
												left: `${limitBoundaryPercent}%`,
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
									// Dotted yellow line marking the soft limit. The default
									// palette yellow is used because the theme has no yellow
									// highlight token.
									<div
										className="absolute -inset-y-1 border-0 border-l-2 border-dotted border-yellow-400"
										style={{ left: `${softMarkerPercent}%` }}
									/>
								)}
								{hardCap === undefined ? (
									// Without a hard cap the allocation is the track's right
									// edge, where its red marker line sits.
									<div className="absolute -inset-y-1 right-0 w-0.5 bg-highlight-red" />
								) : (
									<>
										<div
											className="absolute -inset-y-1 w-0.5 bg-highlight-red"
											style={{ left: `${allocationMarkerPercent}%` }}
										/>
										{/* Double-width line marking the hard cap. It uses the
										    theme's primary content color (white in the dark
										    theme) so it stays visible over the light theme's
										    white card background. */}
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
								{/* The limit label follows its marker so the allocation
								    stays readable on the hard-cap scaled track. Near the
								    right edge it right-aligns to the marker to stay
								    inside the card; near the left edge it renders above
								    the bar instead. */}
								{!limitLabelAboveBar && (
									<p
										className={cn(
											"absolute m-0 text-content-secondary",
											hardCapLabelAboveBar
												? "-translate-x-full"
												: "-translate-x-1/2",
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
