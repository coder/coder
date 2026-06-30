import { normalizeChatUsageLimitPeriod } from "./limitsFormLogic";

describe("limitsFormLogic", () => {
	describe("normalizeChatUsageLimitPeriod", () => {
		it("defaults invalid periods to month", () => {
			expect(normalizeChatUsageLimitPeriod("year")).toBe("month");
			expect(normalizeChatUsageLimitPeriod(undefined)).toBe("month");
		});
	});
});
