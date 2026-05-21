import { describe, expect, it } from "vitest";
import { formatTokenCount } from "./analytics";

describe("formatTokenCount", () => {
	it("formats zero values", () => {
		expect(formatTokenCount(0)).toBe("0");
	});

	it("formats normal values with locale separators", () => {
		expect(formatTokenCount(999)).toBe("999");
		expect(formatTokenCount(1_234)).toBe("1,234");
		expect(formatTokenCount(999_999)).toBe("999,999");
	});

	it("formats large values in millions", () => {
		expect(formatTokenCount(1_000_000)).toBe("1M");
		expect(formatTokenCount(1_500_000)).toBe("1.5M");
		expect(formatTokenCount(2_000_000)).toBe("2M");
	});

	it("rounds million values to one decimal place when needed", () => {
		expect(formatTokenCount(1_250_000)).toBe("1.3M");
	});
});
