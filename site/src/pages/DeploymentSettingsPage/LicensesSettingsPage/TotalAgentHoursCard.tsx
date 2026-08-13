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

	const {
		limit,
		soft_limit: softLimit,
		hard_limit: hardLimit,
		actual,
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
	const usedHours = actual ?? 0;
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
	// "exceeded") flips the bar color. An unlimited allocation has no
	// thresholds to reach.
	const reachedAllocation =
		!isUnlimited && actual !== undefined && usedHours >= meteredLimit;
	const reachedHardCap =
		hardCap !== undefined && actual !== undefined && usedHours >= hardCap;
	// Missing usage data (the usage query failed) must not count as
	// reaching the soft limit: a soft limit of zero is valid and would
	// otherwise compare as reached against the defaulted zero usage.
	const reachedSoftLimit =
		!isUnlimited &&
		!reachedAllocation &&
		actual !== undefined &&
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

	const usedLabel =
		actual === undefined ? "\u2014" : usedHours.toLocaleString("en-US");
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
	// needs the -webkit- prefix for Safari. Usage at or beyond the hard
	// cap swaps the solid red fill for red diagonal stripes.
	const barClassName = isUnlimited
		? cn(
				"bg-[repeating-linear-gradient(-45deg,hsl(var(--highlight-green)),hsl(var(--highlight-green))_6px,transparent_6px,transparent_12px)]",
				"[mask-image:linear-gradient(to_right,black_50%,transparent_100%)]",
				"[-webkit-mask-image:linear-gradient(to_right,black_50%,transparent_100%)]",
			)
		: reachedHardCap
			? "bg-[repeating-linear-gradient(-45deg,hsl(var(--highlight-red)),hsl(var(--highlight-red))_6px,transparent_6px,transparent_12px)]"
			: reachedAllocation
				? "bg-highlight-red"
				: reachedSoftLimit
					? "bg-highlight-orange"
					: "bg-highlight-green";

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

					{hardCapLabelAboveBar && (
						<p className="m-0 self-end text-sm font-medium whitespace-nowrap text-content-secondary">
							Hard cap:{" "}
							<span className="text-content-primary">{hardCapLabel}</span>
						</p>
					)}

					<div
						className="relative h-5 w-full overflow-hidden rounded bg-surface-secondary"
						aria-hidden="true"
					>
						<div
							className={cn(
								"h-full rounded-l transition-[width] duration-300",
								barClassName,
							)}
							style={{ width: `${usagePercentage}%` }}
						/>
						{allocationMarkerPercent !== undefined && (
							// Dotted yellow line marking the allocation on the hard-cap
							// scaled track. The default palette yellow is used because
							// the theme has no yellow highlight token and the soft-limit
							// state already fills the bar with the orange one.
							<div
								className="absolute inset-y-0 border-0 border-l-2 border-dotted border-yellow-400"
								style={{ left: `${allocationMarkerPercent}%` }}
							/>
						)}
						{hardCap !== undefined && (
							// Solid red line marking the hard cap at the track's right
							// edge.
							<div className="absolute inset-y-0 right-0 w-0.5 bg-highlight-red" />
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
								    inside the card. */}
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
