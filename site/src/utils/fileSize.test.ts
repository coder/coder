import { describe, expect, it } from "vitest";
import { formatKiB } from "./fileSize";

describe("formatKiB", () => {
	it("formats bytes as kibibytes with one decimal place", () => {
		expect(formatKiB(0)).toBe("0.0 KiB");
		expect(formatKiB(1536)).toBe("1.5 KiB");
		expect(formatKiB(64 * 1024)).toBe("64.0 KiB");
	});
});
