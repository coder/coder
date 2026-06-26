import { describe, expect, it } from "vitest";
import { getSeverity, severityTextClassName } from "./budget";

describe("getSeverity", () => {
	it("returns normal below the warning threshold", () => {
		expect(getSeverity(0, 50)).toBe("normal");
		expect(getSeverity(42, 50)).toBe("normal");
	});

	it("returns warning at or above 85% of the budget", () => {
		expect(getSeverity(42.5, 50)).toBe("warning");
		expect(getSeverity(46, 50)).toBe("warning");
	});

	it("returns exceeded once usage meets or passes the budget", () => {
		expect(getSeverity(50, 50)).toBe("exceeded");
		expect(getSeverity(75, 50)).toBe("exceeded");
	});

	it("treats a zero budget as exceeded once anything is used", () => {
		expect(getSeverity(0, 0)).toBe("normal");
		expect(getSeverity(5, 0)).toBe("exceeded");
	});

	it("returns normal for non-finite or negative inputs", () => {
		expect(getSeverity(Number.NaN, 50)).toBe("normal");
		expect(getSeverity(10, Number.POSITIVE_INFINITY)).toBe("normal");
		expect(getSeverity(10, -50)).toBe("normal");
	});
});

describe("severityTextClassName", () => {
	it("maps each severity to its text color, defaulting to normal", () => {
		expect(severityTextClassName("exceeded")).toBe("text-content-destructive");
		expect(severityTextClassName("warning")).toBe("text-content-warning");
		expect(severityTextClassName("normal")).toBe("text-content-secondary");
		expect(severityTextClassName()).toBe("text-content-secondary");
	});
});
