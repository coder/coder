import { describe, expect, it } from "vitest";
import {
	deduplicateDiagnostics,
	findUnmemoizedClosureDeps,
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

describe("findUnmemoizedClosureDeps", () => {
	it("detects a bare closure used in a dep check", () => {
		const code = [
			"const urlTransform = url => {",
			"  return rewrite(url);",
			"};",
			"let t0;",
			"if ($[0] !== urlTransform) {",
			"  t0 = <View urlTransform={urlTransform} />;",
			"}",
		].join("\n");
		expect(findUnmemoizedClosureDeps(code)).toEqual([
			{ name: "urlTransform", line: 1 },
		]);
	});

	it("ignores a memoized closure (preceded by else branch)", () => {
		const code = [
			"let t1;",
			"if ($[0] !== proxyHost) {",
			"  t1 = url => rewrite(url, proxyHost);",
			"  $[0] = proxyHost;",
			"  $[1] = t1;",
			"} else {",
			"  t1 = $[1];",
			"}",
			"const urlTransform = t1;",
			"if ($[2] !== urlTransform) {",
			"  t2 = <View urlTransform={urlTransform} />;",
			"}",
		].join("\n");
		expect(findUnmemoizedClosureDeps(code)).toEqual([]);
	});

	it("ignores primitives (not closures)", () => {
		const code = [
			"const offset = (page - 1) * pageSize;",
			"if ($[0] !== offset) {",
			"  t0 = <View offset={offset} />;",
			"}",
		].join("\n");
		expect(findUnmemoizedClosureDeps(code)).toEqual([]);
	});

	it("ignores closures not referenced in any dep check", () => {
		const code = [
			"const handler = () => console.log('hi');",
			"return <View />;",
		].join("\n");
		expect(findUnmemoizedClosureDeps(code)).toEqual([]);
	});

	it("detects async closures", () => {
		const code = [
			"const doWork = async (id) => {",
			"  await api.call(id);",
			"};",
			"if ($[0] !== doWork) {",
			"  t0 = <View handler={doWork} />;",
			"}",
		].join("\n");
		expect(findUnmemoizedClosureDeps(code)).toEqual([
			{ name: "doWork", line: 1 },
		]);
	});

	it("returns empty for empty input", () => {
		expect(findUnmemoizedClosureDeps("")).toEqual([]);
		expect(findUnmemoizedClosureDeps(null)).toEqual([]);
		expect(findUnmemoizedClosureDeps(undefined)).toEqual([]);
	});

	it("detects multiple unmemoized closures", () => {
		const code = [
			"const fn1 = (x) => x + 1;",
			"const fn2 = (y) => y * 2;",
			"if ($[0] !== fn1 || $[1] !== fn2) {",
			"  t0 = <View a={fn1} b={fn2} />;",
			"}",
		].join("\n");
		const result = findUnmemoizedClosureDeps(code);
		expect(result).toHaveLength(2);
		expect(result[0].name).toBe("fn1");
		expect(result[1].name).toBe("fn2");
	});

	// The CLOSURE_RHS regex also matches IIFEs like `const x = (() => {...})();`.
	// The compiler does not emit IIFEs in compiled output, so this is not
	// a real-world false positive today. This test documents the assumption
	// so it breaks visibly if the compiler changes its output shape.
	it("matches IIFEs (documents known regex limitation)", () => {
		const code = [
			"const config = (() => {",
			"  return { theme: 'dark' };",
			"})();",
			"if ($[0] !== config) {",
			"  t0 = <View config={config} />;",
			"}",
		].join("\n");
		// CLOSURE_RHS matches the IIFE because it starts with `(() =>`.
		// This is a known false positive that does not occur in practice.
		expect(findUnmemoizedClosureDeps(code)).toEqual([
			{ name: "config", line: 1 },
		]);
	});
});
