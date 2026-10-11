import { AgentHoursAllotmentMaxBps } from "#/api/typesGenerated";

/** Formats basis points as a percentage, for example 2550 as "25.5%". */
export const formatAllotmentPercent = (bps: number): string =>
	`${(bps / 100).toLocaleString("en-US", { maximumFractionDigits: 2 })}%`;

type ParsedAllotmentPercent =
	| { bps: number }
	| { error: "not-a-number" | "too-many-decimals" };

/**
 * Parses a percentage with at most two decimals into basis points. The digits
 * are combined as integers so values like "0.29" do not drift to 28.
 */
export const parseAllotmentPercent = (
	input: string,
): ParsedAllotmentPercent => {
	const match = /^(\d*)(?:\.(\d*))?$/.exec(input.trim());
	if (!match) {
		return { error: "not-a-number" };
	}
	const [, whole, fraction = ""] = match;
	if (whole === "" && fraction === "") {
		return { error: "not-a-number" };
	}
	if (fraction.length > 2) {
		return { error: "too-many-decimals" };
	}
	return { bps: Number(whole) * 100 + Number(fraction.padEnd(2, "0")) };
};

export const allotmentHours = (
	bps: number,
	poolHours: number | undefined,
): number | undefined =>
	poolHours === undefined
		? undefined
		: (poolHours * bps) / AgentHoursAllotmentMaxBps;

type NamedAllotmentTarget = { id: string; name: string; display_name: string };

/**
 * Display names are not unique, so a target that shares its display name with
 * another target also shows its unique name.
 */
export const allotmentTargetLabel = (
	target: NamedAllotmentTarget,
	targets: readonly NamedAllotmentTarget[],
): string => {
	const label = target.display_name || target.name;
	const collides = targets.some(
		(other) =>
			other.id !== target.id && (other.display_name || other.name) === label,
	);
	return collides && target.name !== label
		? `${label} (${target.name})`
		: label;
};

export const formatHours = (hours: number): string =>
	hours > 0 && hours < 0.01
		? "< 0.01 hours"
		: `${hours.toLocaleString("en-US", { maximumFractionDigits: 2 })} hours`;

type UsedAllotmentTarget = NamedAllotmentTarget & { usedMs: number };

type UsageWithoutAllotmentOptions = {
	/** Every target a label must be told apart from. */
	targets: readonly NamedAllotmentTarget[];
	deletedLabel: string;
	href?: (name: string) => string;
};

/**
 * A target with an empty name was deleted, so it has neither a label of its
 * own nor a page.
 */
export const usageWithoutAllotment = (
	used: readonly UsedAllotmentTarget[],
	allotted: readonly { id: string }[],
	{ targets, deletedLabel, href }: UsageWithoutAllotmentOptions,
) =>
	used
		.filter(
			(target) =>
				target.usedMs > 0 &&
				!allotted.some((allotment) => allotment.id === target.id),
		)
		.map((target) =>
			target.name
				? {
						id: target.id,
						name: allotmentTargetLabel(target, targets),
						usedMs: target.usedMs,
						href: href?.(target.name),
					}
				: { id: target.id, name: deletedLabel, usedMs: target.usedMs },
		);

export const notAttributedMs = (
	totalMs: number,
	organizations: readonly { used_ms: number }[],
): number =>
	// The total and the organization rows come from separate queries, so an
	// hour recorded between them can briefly push the sum above the total.
	Math.max(
		totalMs - organizations.reduce((sum, used) => sum + used.used_ms, 0),
		0,
	);
