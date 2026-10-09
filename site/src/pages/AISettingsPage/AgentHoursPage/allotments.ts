import { AgentHoursAllotmentMaxBps } from "#/api/typesGenerated";

/** Formats basis points as a percentage, for example 2550 as "25.5%". */
export const formatAllotmentPercent = (bps: number): string =>
	`${(bps / 100).toLocaleString("en-US", { maximumFractionDigits: 2 })}%`;

/**
 * Parses a percentage with at most two decimals into basis points. The digits
 * are combined as integers so values like "0.29" do not drift to 28.
 */
export const parseAllotmentPercent = (input: string): number | undefined => {
	const match = /^(\d*)(?:\.(\d{0,2}))?$/.exec(input.trim());
	if (!match) {
		return undefined;
	}
	const [, whole, fraction = ""] = match;
	if (whole === "" && fraction === "") {
		return undefined;
	}
	return Number(whole) * 100 + Number(fraction.padEnd(2, "0"));
};

export const allotmentHours = (
	bps: number,
	poolHours: number | undefined,
): number | undefined =>
	poolHours === undefined
		? undefined
		: (poolHours * bps) / AgentHoursAllotmentMaxBps;

export const formatHours = (hours: number): string =>
	hours > 0 && hours < 0.01
		? "< 0.01 hours"
		: `${hours.toLocaleString("en-US", { maximumFractionDigits: 2 })} hours`;
