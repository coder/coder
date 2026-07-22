import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
	buildRules,
	collectFiles,
	discoverQueryKeySegments,
	findImprovements,
	findIncreases,
	matchOccurrences,
	runCli,
	scan,
} from "./check-frontend-patterns.mjs";

describe("discoverQueryKeySegments", () => {
	it("finds plain array key constants", () => {
		const segs = discoverQueryKeySegments([
			`const chatModelConfigsKey = ["chat-model-configs"] as const;`,
		]);
		expect(segs).toEqual(new Set(["chat-model-configs"]));
	});

	it("finds single-line arrow-function key helpers", () => {
		const segs = discoverQueryKeySegments([
			`export const chatKey = (chatId: string) => ["chats", chatId] as const;`,
		]);
		expect(segs).toEqual(new Set(["chats"]));
	});

	it("finds multi-line arrow-function key helpers", () => {
		const segs = discoverQueryKeySegments([
			`export const chatMessagesKey = (chatId: string) =>\n\t["chats", chatId, "messages"] as const;`,
		]);
		expect(segs).toEqual(new Set(["chats"]));
	});

	it("finds UPPER_CASE key constants", () => {
		const segs = discoverQueryKeySegments([
			`export const HEALTH_QUERY_KEY = ["health"];`,
		]);
		expect(segs).toEqual(new Set(["health"]));
	});

	it("finds inline queryKey arrays in query option helpers", () => {
		const segs = discoverQueryKeySegments([
			`export const portForward = (agentId: string) => ({\n\tqueryKey: ["portForward", agentId],\n\tqueryFn: f,\n});`,
		]);
		expect(segs).toEqual(new Set(["portForward"]));
	});

	it("resolves same-file string constants used as queryKey roots", () => {
		const segs = discoverQueryKeySegments([
			`export const templateVersionRoot: string = "templateVersion";\nexport const templateVersion = (id: string) => ({\n\tqueryKey: [templateVersionRoot, id],\n\tqueryFn: f,\n});`,
		]);
		expect(segs).toEqual(new Set(["templateVersion"]));
	});

	it("ignores constants whose first element is not a string literal", () => {
		const segs = discoverQueryKeySegments([
			`export const getAuthorizationKey = (req: AuthorizationRequest) =>\n\t[AUTHORIZATION_KEY, req] as const;`,
		]);
		expect(segs).toEqual(new Set());
	});

	it("ignores non-key constants", () => {
		const segs = discoverQueryKeySegments([
			`const items = ["a", "b"];\nconst path = ["nested"];`,
		]);
		expect(segs).toEqual(new Set());
	});
});

describe("matchOccurrences", () => {
	it("returns 1-based line numbers and trimmed line text per match", () => {
		expect(
			matchOccurrences("a\n  el.querySelector(x);\nb", /querySelector\(/g),
		).toEqual([{ line: 2, text: "el.querySelector(x);" }]);
	});

	it("normalizes em and en dashes so the baseline stays ASCII", () => {
		expect(
			matchOccurrences(
				"try { f(); } catch { /* boom \u2014 ignore \u2013 fine */ }",
				/catch\s*\{[^}]*\}/g,
			),
		).toEqual([
			{ line: 1, text: "try { f(); } catch { /* boom - ignore - fine */ }" },
		]);
	});
});

// Write fixture files into a temp site dir and return its path.
const makeSite = (files) => {
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), "fe-patterns-"));
	for (const [rel, content] of Object.entries(files)) {
		const full = path.join(dir, rel);
		fs.mkdirSync(path.dirname(full), { recursive: true });
		fs.writeFileSync(full, content);
	}
	return dir;
};

describe("scan", () => {
	const rules = buildRules(new Set(["chats"]));

	it("flags querySelector in stories and tests but not in components", () => {
		const dir = makeSite({
			"src/A.stories.tsx": `canvasElement.querySelector(".x");`,
			"src/A.test.tsx": `el.querySelector(".x");`,
			"src/A.tsx": `el.querySelector(".x");`,
		});
		const files = collectFiles(path.join(dir, "src"), dir);
		const { occurrences } = scan(dir, files, rules);
		expect(occurrences["FE10/no-queryselector-in-ui-tests"]).toEqual({
			[path.join("src", "A.stories.tsx")]: ['canvasElement.querySelector(".x");'],
			[path.join("src", "A.test.tsx")]: ['el.querySelector(".x");'],
		});
	});

	it("flags class-attribute selectors in stories and tests", () => {
		const dir = makeSite({
			"src/B.stories.tsx": `canvas.querySelectorAll("[class*='flex']");`,
			"src/B.test.tsx": `screen.getByTestId("x").closest("[class^=btn]");`,
		});
		const files = collectFiles(path.join(dir, "src"), dir);
		const { occurrences } = scan(dir, files, rules);
		expect(occurrences["FE10/no-class-substring-selectors"]).toEqual({
			[path.join("src", "B.stories.tsx")]: [
				`canvas.querySelectorAll("[class*='flex']");`,
			],
			[path.join("src", "B.test.tsx")]: [
				`screen.getByTestId("x").closest("[class^=btn]");`,
			],
		});
	});

	it("flags re-typed query keys only for known segments", () => {
		const dir = makeSite({
			"src/C.stories.tsx": `parameters: { queries: [{ key: ["chats", "1"], data: {} }, { key: ["unknown"], data: {} }] }`,
		});
		const files = collectFiles(path.join(dir, "src"), dir);
		const { occurrences, details } = scan(dir, files, rules);
		expect(occurrences["FE7/no-retyped-query-keys"]).toEqual({
			[path.join("src", "C.stories.tsx")]: [
				`parameters: { queries: [{ key: ["chats", "1"], data: {} }, { key: ["unknown"], data: {} }] }`,
			],
		});
		expect(details[0]).toContain("re-typed query key");
	});

	it("gives wrapped multi-line keys a distinctive signature", () => {
		const dir = makeSite({
			"src/C3.stories.tsx": `parameters: {\n\tqueries: [\n\t\t{\n\t\t\tkey: [\n\t\t\t\t"chats",\n\t\t\t\t"1",\n\t\t\t],\n\t\t\tdata: {},\n\t\t},\n\t],\n}`,
		});
		const files = collectFiles(path.join(dir, "src"), dir);
		const { occurrences } = scan(dir, files, rules);
		// Not a bare "key: [": the normalized match keeps the first segment.
		expect(occurrences["FE7/no-retyped-query-keys"]).toEqual({
			[path.join("src", "C3.stories.tsx")]: [`key: [ "chats"`],
		});
	});

	it("flags re-typed keys in react-query queryKey options", () => {
		const dir = makeSite({
			"src/C2.test.tsx": `useQuery({ queryKey: ["chats", "1"], queryFn: f });\nconst monkey: ["chats"] = x;`,
		});
		const files = collectFiles(path.join(dir, "src"), dir);
		const { occurrences } = scan(dir, files, rules);
		expect(occurrences["FE7/no-retyped-query-keys"]).toEqual({
			[path.join("src", "C2.test.tsx")]: [
				`useQuery({ queryKey: ["chats", "1"], queryFn: f });`,
			],
		});
	});

	it("flags queryKey literals with brand-new segments, but keeps the segment guard for key:", () => {
		const dir = makeSite({
			"src/pages/New.tsx": `useQuery({ queryKey: ["newThing"], queryFn: f });`,
			"src/Obj.stories.tsx": `const columns = [{ key: ["not-a-query"], label: "x" }];`,
		});
		const files = collectFiles(path.join(dir, "src"), dir);
		const { occurrences } = scan(dir, files, rules);
		expect(occurrences["FE7/no-retyped-query-keys"]).toEqual({
			[path.join("src", "pages", "New.tsx")]: [
				`useQuery({ queryKey: ["newThing"], queryFn: f });`,
			],
		});
	});

	it("flags re-typed keys in components but not in api/queries", () => {
		const dir = makeSite({
			"src/pages/Foo.tsx": `useQuery({ queryKey: ["chats"], queryFn: f });`,
			"src/api/queries/chats2.ts": `export const q = () => ({ queryKey: ["chats"], queryFn: f });`,
		});
		const files = collectFiles(path.join(dir, "src"), dir);
		const { occurrences } = scan(dir, files, rules);
		expect(occurrences["FE7/no-retyped-query-keys"]).toEqual({
			[path.join("src", "pages", "Foo.tsx")]: [
				`useQuery({ queryKey: ["chats"], queryFn: f });`,
			],
		});
	});

	it("flags empty catch outside tests, allows non-empty catch", () => {
		const dir = makeSite({
			"src/D.tsx": `try { f(); } catch {}\ntry { g(); } catch (e) { log(e); }`,
			"src/D.test.tsx": `try { f(); } catch {}`,
		});
		const files = collectFiles(path.join(dir, "src"), dir);
		const { occurrences } = scan(dir, files, rules);
		expect(occurrences["FE7/no-empty-catch"]).toEqual({
			[path.join("src", "D.tsx")]: ["try { f(); } catch {}"],
		});
	});

	it("flags comment-only catch bodies", () => {
		const dir = makeSite({
			"src/E.tsx": [
				`try { f(); } catch { /* ignore */ }`,
				`try { g(); } catch (e) {`,
				`\t// nothing to do`,
				`}`,
				`try { h(); } catch { /* note */ recover(); }`,
			].join("\n"),
		});
		const files = collectFiles(path.join(dir, "src"), dir);
		const { occurrences } = scan(dir, files, rules);
		// The multi-line catch gets the normalized match text; the
		// single-line one gets the full trimmed line. Arrays are sorted.
		expect(occurrences["FE7/no-empty-catch"]).toEqual({
			[path.join("src", "E.tsx")]: [
				"catch (e) { // nothing to do }",
				"try { f(); } catch { /* ignore */ }",
			],
		});
	});
});

describe("findIncreases / findImprovements", () => {
	const rules = [{ id: "r1" }, { id: "r2" }];

	it("reports new occurrences, including duplicates and new files", () => {
		const baseline = { r1: { "a.tsx": ["x", "x"] } };
		const occurrences = { r1: { "a.tsx": ["x", "x", "x"], "b.tsx": ["y"] }, r2: {} };
		expect(findIncreases(baseline, occurrences, rules)).toEqual([
			"a.tsx  r1: x",
			"b.tsx  r1: y",
		]);
	});

	it("flags a same-file swap even when the total count is unchanged", () => {
		const baseline = { r1: { "a.tsx": ["old violation"] } };
		const occurrences = { r1: { "a.tsx": ["new violation"] }, r2: {} };
		expect(findIncreases(baseline, occurrences, rules)).toEqual([
			"a.tsx  r1: new violation",
		]);
		expect(findImprovements(baseline, occurrences, rules)).toEqual(["a.tsx"]);
	});

	it("reports removed occurrences, including fully fixed files", () => {
		const baseline = { r1: { "a.tsx": ["x", "x"], "b.tsx": ["y"] } };
		const occurrences = { r1: { "a.tsx": ["x"] }, r2: {} };
		expect(findImprovements(baseline, occurrences, rules)).toEqual([
			"a.tsx",
			"b.tsx",
		]);
	});

	it("returns nothing when occurrences match the baseline", () => {
		const baseline = { r1: { "a.tsx": ["x"] }, r2: {} };
		const occurrences = { r1: { "a.tsx": ["x"] }, r2: {} };
		expect(findIncreases(baseline, occurrences, rules)).toEqual([]);
		expect(findImprovements(baseline, occurrences, rules)).toEqual([]);
	});
});

describe("runCli", () => {
	const cli = (siteDir, argv) => {
		const logs = [];
		const errors = [];
		const code = runCli({
			siteDir,
			baselinePath: path.join(siteDir, "baseline.json"),
			argv,
			log: (m) => logs.push(m),
			error: (m) => errors.push(m),
		});
		return { code, logs, errors };
	};

	const fixture = {
		"src/api/queries/chats.ts": `const chatsKey = ["chats"] as const;`,
		"src/Ok.stories.tsx": `within(canvasElement).getByRole("button");`,
	};

	it("fails without a baseline and passes after --update creates one", () => {
		const dir = makeSite(fixture);
		expect(cli(dir, []).code).toBe(1);
		expect(cli(dir, ["--update"]).code).toBe(0);
		expect(cli(dir, []).code).toBe(0);
	});

	it("fails on a new violation with file and line details", () => {
		const dir = makeSite(fixture);
		cli(dir, ["--update"]);
		fs.writeFileSync(
			path.join(dir, "src", "Bad.stories.tsx"),
			`canvasElement.querySelector(".x");`,
		);
		const { code, errors } = cli(dir, []);
		expect(code).toBe(1);
		expect(errors.join("\n")).toContain("Bad.stories.tsx");
		expect(errors.join("\n")).toContain("FE10/no-queryselector-in-ui-tests");
	});

	it("refuses --update when violations increase, unless --allow-increase", () => {
		const dir = makeSite(fixture);
		cli(dir, ["--update"]);
		fs.writeFileSync(
			path.join(dir, "src", "Bad.stories.tsx"),
			`canvasElement.querySelector(".x");`,
		);
		const refused = cli(dir, ["--update"]);
		expect(refused.code).toBe(1);
		expect(refused.errors.join("\n")).toContain("Refusing to raise the baseline");
		expect(cli(dir, ["--update", "--allow-increase"]).code).toBe(0);
		expect(cli(dir, []).code).toBe(0);
	});

	it("prompts to ratchet down when violations are fixed", () => {
		const dir = makeSite({
			...fixture,
			"src/Bad.stories.tsx": `canvasElement.querySelector(".x");`,
		});
		cli(dir, ["--update"]);
		fs.writeFileSync(
			path.join(dir, "src", "Bad.stories.tsx"),
			`within(canvasElement).getByRole("button");`,
		);
		const { code, errors } = cli(dir, []);
		expect(code).toBe(1);
		expect(errors.join("\n")).toContain("Ratchet the baseline down");
		expect(cli(dir, ["--update"]).code).toBe(0);
		expect(cli(dir, []).code).toBe(0);
	});
});
