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

const msPerTenthHour = 360_000;

/**
 * Converts Agent Time to hours, floored to tenths like the license usage card
 * so the figures here never read higher than the license total.
 */
export const usedHours = (ms: number): number =>
	Number.isFinite(ms) ? Math.floor(ms / msPerTenthHour) / 10 : 0;

export const formatUsedHours = (ms: number): string =>
	usedHours(ms).toLocaleString("en-US", {
		minimumFractionDigits: 1,
		maximumFractionDigits: 1,
	});

/** The hours of a pool that no allotment claims, undefined for an unknown pool. */
export const remainderHours = (
	allottedBps: number,
	poolHours: number | undefined,
): number | undefined =>
	allotmentHours(
		Math.max(AgentHoursAllotmentMaxBps - allottedBps, 0),
		poolHours,
	);
