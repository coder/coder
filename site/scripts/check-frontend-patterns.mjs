/**
 * Deterministic checks for a subset of the frontend pattern rules in
 * .claude/docs/FRONTEND_PATTERNS.md. Only patterns that are reliably
 * detectable with text matching are checked here; the rest are covered
 * by review.
 *
 * Existing violations are tracked in frontend-patterns-baseline.json as
 * a ratchet keyed by occurrence signature (the trimmed text of each
 * matching line, as a multiset per file): new or changed violations
 * fail even when an old one is removed in the same edit, and fixing
 * violations requires updating the baseline so the allowed set only
 * shrinks. --update refuses increases unless --allow-increase is
 * passed (for deliberate cases such as newly added rules).
 *
 * Usage:  node scripts/check-frontend-patterns.mjs [--update [--allow-increase]]
 */
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const isStory = (f) => f.endsWith(".stories.tsx");
const isTest = (f) => /\.(test|spec)\.tsx?$/.test(f);

export function collectFiles(dir, rootDir = dir) {
	const results = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const full = join(dir, entry.name);
		if (entry.isDirectory()) {
			results.push(...collectFiles(full, rootDir));
		} else if (entry.name.endsWith(".ts") || entry.name.endsWith(".tsx")) {
			results.push(relative(rootDir, full));
		}
	}
	return results;
}

// Three shapes contribute segments: named key constants/helpers (plain
// arrays, arrow functions single- or multi-line, UPPER_CASE names),
// inline queryKey arrays (for example `queryKey: ["portForward", agentId]`
// with no named constant), and queryKey arrays whose first element is a
// same-file string constant (for example `templateVersionRoot =
// "templateVersion"` used as `queryKey: [templateVersionRoot, id]`).
export function discoverQueryKeySegments(sources) {
	const segments = new Set();
	for (const src of sources) {
		const stringConsts = new Map();
		for (const m of src.matchAll(
			/const\s+(\w+)(?:\s*:\s*\w+)?\s*=\s*"([^"]+)"/g,
		)) {
			stringConsts.set(m[1], m[2]);
		}
		for (const m of src.matchAll(
			/const\s+\w*(?:[Kk]ey|KEY)\w*\s*=[^;\][]*\[\s*"([^"]+)"/g,
		)) {
			segments.add(m[1]);
		}
		for (const m of src.matchAll(/\bqueryKey:\s*\[\s*(?:"([^"]+)"|(\w+))/g)) {
			if (m[1]) {
				segments.add(m[1]);
			} else if (stringConsts.has(m[2])) {
				segments.add(stringConsts.get(m[2]));
			}
		}
	}
	return segments;
}

export function lineOf(src, index) {
	return src.slice(0, index).split("\n").length;
}

// Occurrence signature for a regex match: the trimmed text of the first
// matching line, or the whitespace-normalized match text when the match
// spans multiple lines (a lone "key: [" line is not distinctive, but the
// normalized match includes the first string segment). Signatures survive
// line-number shifts from unrelated edits, unlike line numbers, and
// distinguish a new violation from a baselined one, unlike counts.
export function signatureFor(src, sourceLines, m) {
	const startLine = lineOf(src, m.index);
	const endLine = lineOf(src, m.index + m[0].length - 1);
	const text =
		endLine > startLine
			? m[0].replace(/\s+/g, " ").trim()
			: sourceLines[startLine - 1].trim();
	// The repo-wide emdash lint rejects added lines containing U+2014 or
	// U+2013, and the baseline is a committed file; keep signatures ASCII.
	return text.replace(/[\u2013\u2014]/g, "-");
}

// Returns { line, text } per match, where text is the occurrence signature.
export function matchOccurrences(src, regex) {
	const sourceLines = src.split("\n");
	const occurrences = [];
	for (const m of src.matchAll(regex)) {
		occurrences.push({
			line: lineOf(src, m.index),
			text: signatureFor(src, sourceLines, m),
		});
	}
	return occurrences;
}

export function buildRules(queryKeySegments) {
	return [
		{
			id: "FE10/no-queryselector-in-ui-tests",
			files: (f) => isStory(f) || isTest(f),
			find: (src) => matchOccurrences(src, /\bquerySelector(All)?\s*\(/g),
			message:
				"querySelector in a story or test; query by role or accessible name",
		},
		{
			id: "FE10/no-class-substring-selectors",
			files: (f) => isStory(f) || isTest(f),
			find: (src) => matchOccurrences(src, /\[class[*^$]?=/g),
			message: "class-attribute selector in a test; assert observable behavior",
		},
		{
			id: "FE7/no-retyped-query-keys",
			// Everywhere except api/queries, where key constants and query
			// option helpers legitimately define the literals.
			files: (f) => !f.startsWith(join("src", "api", "queries")),
			find: (src) => {
				const sourceLines = src.split("\n");
				const occurrences = [];
				// queryKey: is react-query API, so a string-literal key outside
				// api/queries is always a violation, including brand-new segments.
				// key: is a generic property name (story query-mock wiring but
				// also arbitrary objects), so it is flagged only when the first
				// segment matches a discovered query key.
				for (const m of src.matchAll(/\b(queryKey|key):\s*\[\s*"([^"]+)"/g)) {
					if (m[1] === "queryKey" || queryKeySegments.has(m[2])) {
						occurrences.push({
							line: lineOf(src, m.index),
							text: signatureFor(src, sourceLines, m),
						});
					}
				}
				return occurrences;
			},
			message:
				"re-typed query key string; import the key constant from api/queries",
		},
		{
			id: "FE7/no-empty-catch",
			files: (f) => !isTest(f),
			// A catch body containing only whitespace and comments still
			// swallows the error; a placeholder comment does not handle it.
			find: (src) =>
				matchOccurrences(
					src,
					/catch\s*(\([^)]*\))?\s*\{(?:\s|\/\/[^\n]*|\/\*[\s\S]*?\*\/)*\}/g,
				),
			message: "empty catch block silently swallows errors",
		},
	];
}

// Returns { occurrences, details } where occurrences[ruleId][file] is a
// sorted array of occurrence signatures (trimmed matched-line texts,
// duplicates preserved).
export function scan(siteDir, files, rules) {
	const occurrences = {};
	const details = [];
	for (const rule of rules) {
		occurrences[rule.id] = {};
		for (const file of files.filter(rule.files)) {
			const src = readFileSync(join(siteDir, file), "utf8");
			const found = rule.find(src);
			if (found.length > 0) {
				occurrences[rule.id][file] = found.map((o) => o.text).sort();
				for (const o of found) {
					details.push(`${file}:${o.line}  ${rule.id}  ${rule.message}`);
				}
			}
		}
	}
	return { occurrences, details };
}

const toMultiset = (texts) => {
	const counts = new Map();
	for (const t of texts ?? []) {
		counts.set(t, (counts.get(t) ?? 0) + 1);
	}
	return counts;
};

// Occurrences present now but absent from (or more frequent than in) the
// baseline. A same-file swap of one violation for a different one is an
// increase even though the total count is unchanged.
export function findIncreases(baseline, occurrences, rules) {
	const increases = [];
	for (const rule of rules) {
		const baseRule = baseline[rule.id] ?? {};
		for (const [file, texts] of Object.entries(occurrences[rule.id] ?? {})) {
			const allowed = toMultiset(baseRule[file]);
			for (const [text, count] of toMultiset(texts)) {
				if (count > (allowed.get(text) ?? 0)) {
					increases.push(`${file}  ${rule.id}: ${text}`);
				}
			}
		}
	}
	return increases;
}

// Files with baselined occurrences that no longer exist.
export function findImprovements(baseline, occurrences, rules) {
	const improvements = [];
	for (const rule of rules) {
		const baseRule = baseline[rule.id] ?? {};
		const currRule = occurrences[rule.id] ?? {};
		for (const [file, texts] of Object.entries(baseRule)) {
			const current = toMultiset(currRule[file]);
			const gone = [...toMultiset(texts)].some(
				([text, count]) => (current.get(text) ?? 0) < count,
			);
			if (gone) {
				improvements.push(file);
			}
		}
	}
	return improvements;
}

export function runCli({ siteDir, baselinePath, argv, log, error }) {
	const update = argv.includes("--update");
	const srcDir = join(siteDir, "src");
	const files = collectFiles(srcDir, siteDir);

	const queryFiles = files.filter((f) =>
		f.startsWith(join("src", "api", "queries")),
	);
	const queryKeySegments = discoverQueryKeySegments(
		queryFiles.map((f) => readFileSync(join(siteDir, f), "utf8")),
	);
	const rules = buildRules(queryKeySegments);
	const { occurrences, details } = scan(siteDir, files, rules);

	if (update) {
		let existing = null;
		try {
			existing = JSON.parse(readFileSync(baselinePath, "utf8"));
		} catch {
			// No baseline yet; write the initial one below.
		}
		if (existing && !argv.includes("--allow-increase")) {
			const increases = findIncreases(existing, occurrences, rules);
			if (increases.length > 0) {
				error(
					"Refusing to raise the baseline. Fix these violations instead:\n",
				);
				for (const i of increases) {
					error(`  ${i}`);
				}
				error(
					"\nIf an increase is deliberate (for example, a newly added rule), pass --allow-increase.",
				);
				return 1;
			}
		}
		writeFileSync(baselinePath, `${JSON.stringify(occurrences, null, "\t")}\n`);
		log(`Baseline updated: ${relative(siteDir, baselinePath)}`);
		return 0;
	}

	let baseline = null;
	try {
		baseline = JSON.parse(readFileSync(baselinePath, "utf8"));
	} catch {
		error(
			"Missing or unreadable baseline. Run: node scripts/check-frontend-patterns.mjs --update",
		);
		return 1;
	}

	const regressions = findIncreases(baseline, occurrences, rules);
	if (regressions.length > 0) {
		error(
			"New frontend pattern violations (see .claude/docs/FRONTEND_PATTERNS.md):\n",
		);
		for (const r of regressions) {
			error(`  ${r}`);
		}
		error("\nMatching occurrences in those files:\n");
		const flagged = new Set(regressions.map((r) => r.split("  ")[0]));
		for (const d of details.filter((d) => flagged.has(d.split(":")[0]))) {
			error(`  ${d}`);
		}
		return 1;
	}

	const improvements = findImprovements(baseline, occurrences, rules);
	if (improvements.length > 0) {
		error(
			"Baselined violations were fixed (nice!). Ratchet the baseline down:\n\n  node scripts/check-frontend-patterns.mjs --update\n",
		);
		return 1;
	}

	log("Frontend pattern checks passed.");
	return 0;
}

// Keep CLI behavior out of module import so the exports are testable.
if (process.argv[1] === fileURLToPath(import.meta.url)) {
	const siteDir = fileURLToPath(new URL("..", import.meta.url));
	process.exit(
		runCli({
			siteDir,
			baselinePath: join(siteDir, "scripts", "frontend-patterns-baseline.json"),
			argv: process.argv.slice(2),
			log: console.info,
			error: console.error,
		}),
	);
}
