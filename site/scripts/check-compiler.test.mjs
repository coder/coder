import { describe, expect, it } from "vitest";
import {
	deduplicateDiagnostics,
	shortPath,
	shortenMessage,
} from "./check-compiler.mjs";

describe("shortenMessage", () => {
	it("strips Error: prefix and takes first sentence", () => {
		expect(
			shortenMessage(
				"Error: Ref values are not allowed. Use ref types instead.",
			),
		).toBe("Ref values are not allowed");
	});

	it("strips trailing URL references", () => {
		expect(
			shortenMessage("Mutating a value returned from a hook(https://react.dev/reference)"),
		).toBe("Mutating a value returned from a hook");
	});

	it("preserves dotted property paths", () => {
		expect(
			shortenMessage("Cannot destructure props.foo because it is null"),
		).toBe("Cannot destructure props.foo because it is null");
	});

	it("coerces non-string values", () => {
		expect(shortenMessage(42)).toBe("42");
		expect(shortenMessage({ toString: () => "Error: obj. detail" })).toBe("obj");
	});

	it("normalizes trailing periods", () => {
		expect(shortenMessage("Single sentence.")).toBe("Single sentence");
	});

	it("preserves empty string and (unknown) sentinel", () => {
		expect(shortenMessage("")).toBe("");
		expect(shortenMessage("(unknown)")).toBe("(unknown)");
	});
});

describe("deduplicateDiagnostics", () => {
	it("removes duplicates with same line and message", () => {
		const input = [
			{ line: 1, short: "error A" },
			{ line: 1, short: "error A" },
			{ line: 2, short: "error B" },
		];
		expect(deduplicateDiagnostics(input)).toEqual([
			{ line: 1, short: "error A" },
			{ line: 2, short: "error B" },
		]);
	});

	it("keeps diagnostics with same message on different lines", () => {
		const input = [
			{ line: 1, short: "error A" },
			{ line: 2, short: "error A" },
		];
		expect(deduplicateDiagnostics(input)).toEqual(input);
	});

	it("keeps diagnostics with same line but different messages", () => {
		const input = [
			{ line: 1, short: "error A" },
			{ line: 1, short: "error B" },
		];
		expect(deduplicateDiagnostics(input)).toEqual(input);
	});

	it("returns empty array for empty input", () => {
		expect(deduplicateDiagnostics([])).toEqual([]);
	});
});

describe("shortPath", () => {
	const dirs = ["src/pages/AgentsPage", "src/pages/Other"];

	it("strips matching target dir prefix", () => {
		expect(shortPath("src/pages/AgentsPage/components/Chat.tsx", dirs))
			.toBe("components/Chat.tsx");
	});

	it("strips first matching prefix when multiple match", () => {
		expect(shortPath("src/pages/Other/index.tsx", dirs))
			.toBe("index.tsx");
	});

	it("returns file unchanged when no prefix matches", () => {
		expect(shortPath("src/utils/helper.ts", dirs))
			.toBe("src/utils/helper.ts");
	});
});
