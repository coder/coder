import { describe, expect, it } from "vitest";
import {
	computeNewViolations,
	countSignatures,
	findViolationsInText,
	hintFor,
	signatureOf,
} from "./check-design-tokens.mjs";

describe("findViolationsInText", () => {
	it("flags raw hex arbitrary colors", () => {
		const v = findViolationsInText(`<div className="text-[#F87171]" />`);
		expect(v).toEqual([{ rule: "color", snippet: "text-[#F87171]", line: 1 }]);
	});

	it("flags arbitrary color functions", () => {
		const v = findViolationsInText(`<div className="bg-[rgb(0,0,0)]" />`);
		expect(v.map((x) => x.rule)).toEqual(["color"]);
		expect(v[0].snippet).toBe("bg-[rgb(");
	});

	it("flags default palette shades", () => {
		const v = findViolationsInText(`<div className="text-red-500 bg-gray-100" />`);
		expect(v.map((x) => x.snippet).sort()).toEqual([
			"bg-gray-100",
			"text-red-500",
		]);
	});

	it("allows semantic color tokens", () => {
		const v = findViolationsInText(
			`<div className="text-content-primary bg-surface-secondary border-border" />`,
		);
		expect(v).toEqual([]);
	});

	it("flags arbitrary font sizes", () => {
		const v = findViolationsInText(
			`<span className="text-[13px]" /><span className="text-[0.8rem]" />`,
		);
		expect(v.map((x) => x.snippet)).toEqual(["text-[13px]", "text-[0.8rem]"]);
		expect(v.every((x) => x.rule === "font")).toBe(true);
	});

	it("allows the token type scale and dynamic length values", () => {
		const v = findViolationsInText(
			`<span className="text-sm text-2xs text-[length:inherit] text-[length:var(--x)]" />`,
		);
		expect(v).toEqual([]);
	});

	it("flags arbitrary spacing lengths", () => {
		const v = findViolationsInText(
			`<div className="p-[7px] gap-[18px] mb-[0.75em]" />`,
		);
		expect(v.map((x) => x.snippet).sort()).toEqual([
			"gap-[18px]",
			"mb-[0.75em]",
			"p-[7px]",
		]);
		expect(v.every((x) => x.rule === "spacing")).toBe(true);
	});

	it("allows the spacing scale and var/calc/keyword values", () => {
		const v = findViolationsInText(
			`<div className="p-2 gap-4 pt-[calc(var(--a)+var(--b))] ml-[initial]" />`,
		);
		expect(v).toEqual([]);
	});

	it("reports the correct line number", () => {
		const v = findViolationsInText(`line one\n<div className="p-[7px]" />`);
		expect(v).toEqual([{ rule: "spacing", snippet: "p-[7px]", line: 2 }]);
	});
});

describe("signatureOf / countSignatures", () => {
	it("builds a line-independent signature", () => {
		expect(signatureOf({ rule: "spacing", snippet: "p-[7px]", line: 9 })).toBe(
			"spacing|p-[7px]",
		);
	});

	it("counts repeated signatures", () => {
		const counts = countSignatures([
			{ rule: "spacing", snippet: "p-[7px]", line: 1 },
			{ rule: "spacing", snippet: "p-[7px]", line: 4 },
			{ rule: "font", snippet: "text-[13px]", line: 2 },
		]);
		expect(counts).toEqual({ "spacing|p-[7px]": 2, "font|text-[13px]": 1 });
	});
});

describe("hintFor", () => {
	it("returns a hint per known rule", () => {
		expect(hintFor("color")).toContain("semantic color token");
		expect(hintFor("font")).toContain("--text-*");
		expect(hintFor("spacing")).toContain("4px spacing scale");
	});

	it("returns an empty string for unknown rules", () => {
		expect(hintFor("nope")).toBe("");
	});
});

describe("computeNewViolations", () => {
	it("returns nothing when counts stay within the baseline", () => {
		const current = {
			"a.tsx": [{ rule: "spacing", snippet: "p-[7px]", line: 3 }],
		};
		const baseline = { "a.tsx": { "spacing|p-[7px]": 1 } };
		expect(computeNewViolations(current, baseline)).toEqual([]);
	});

	it("flags occurrences beyond the baseline count", () => {
		const current = {
			"a.tsx": [
				{ rule: "spacing", snippet: "p-[7px]", line: 3 },
				{ rule: "spacing", snippet: "p-[7px]", line: 8 },
			],
		};
		const baseline = { "a.tsx": { "spacing|p-[7px]": 1 } };
		expect(computeNewViolations(current, baseline)).toEqual([
			{ path: "a.tsx", rule: "spacing", snippet: "p-[7px]", line: 8 },
		]);
	});

	it("flags all occurrences in a file absent from the baseline", () => {
		const current = {
			"new.tsx": [{ rule: "font", snippet: "text-[13px]", line: 2 }],
		};
		expect(computeNewViolations(current, {})).toEqual([
			{ path: "new.tsx", rule: "font", snippet: "text-[13px]", line: 2 },
		]);
	});

	it("does not fail when a baseline entry is removed (ratchet down)", () => {
		const current = {};
		const baseline = { "a.tsx": { "spacing|p-[7px]": 2 } };
		expect(computeNewViolations(current, baseline)).toEqual([]);
	});

	it("sorts results by path then line", () => {
		const current = {
			"b.tsx": [{ rule: "font", snippet: "text-[13px]", line: 5 }],
			"a.tsx": [{ rule: "spacing", snippet: "p-[7px]", line: 9 }],
		};
		const result = computeNewViolations(current, {});
		expect(result.map((r) => r.path)).toEqual(["a.tsx", "b.tsx"]);
	});
});
