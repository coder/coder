import type { ChatUsageLimitPeriod } from "#/api/typesGenerated";

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
