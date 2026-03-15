import type { ChatUsageLimitPeriod } from "api/typesGenerated";

export const microsToDollars = (micros: number): number =>
	Math.round(micros / 10_000) / 100;

export const dollarsToMicros = (dollars: string): number => {
	const result = Math.round(Number(dollars) * 1_000_000);
	return Number.isFinite(result) && result > 0 ? result : 0;
};

export const isPositiveFiniteDollarAmount = (dollars: string): boolean => {
	const parsed = Number(dollars);
	return (
		dollars.trim() !== "" &&
		Number.isFinite(parsed) &&
		parsed > 0 &&
		Math.round(parsed * 1_000_000) >= 1
	);
};

export const normalizeChatUsageLimitPeriod = (
	period: string | null | undefined,
): ChatUsageLimitPeriod => {
	switch (period) {
		case "day":
		case "week":
		case "month":
			return period;
		default:
			return "month";
	}
};
