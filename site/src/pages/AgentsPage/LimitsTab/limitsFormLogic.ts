import type { ChatUsageLimitPeriod } from "api/typesGenerated";

export const microsToDollars = (micros: number): number =>
	Math.round(micros / 10_000) / 100;

export const dollarsToMicros = (dollars: string): number =>
	Math.round(Number(dollars) * 1_000_000);

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
