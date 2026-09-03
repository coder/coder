/**
 * Design token linter.
 *
 * Scans .ts/.tsx sources for Tailwind utilities that bypass the design
 * system defined in src/index.css and enforces three rules:
 *
 *   color   - only semantic color tokens (e.g. text-content-primary,
 *             bg-surface-secondary). Raw hex/rgb/hsl arbitrary values and
 *             the default Tailwind palette (text-red-500, bg-gray-100, ...)
 *             are rejected.
 *   font    - font sizes must come from the --text-* scale. Arbitrary
 *             values such as text-[13px] or text-[0.8rem] are rejected.
 *   spacing - padding/margin/gap/space must use the 4px spacing scale.
 *             Arbitrary length values such as p-[7px] or gap-[18px] are
 *             rejected. var()/calc()/keyword values are allowed.
 *
 * The rules run against the whole codebase, which already contains
 * pre-existing occurrences. To adopt the rules without a large upfront
 * migration we compare against a baseline (design-tokens-baseline.json):
 * only NEW occurrences beyond the recorded counts fail. Run with --update
 * to regenerate the baseline after intentionally fixing or adding entries.
 *
 * Usage:
 *   node scripts/check-design-tokens.mjs            # check, exit 1 on new violations
 *   node scripts/check-design-tokens.mjs --update   # rewrite the baseline
 */
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

// Resolve the site/ directory (ESM equivalent of __dirname + "..").
const siteDir = new URL("..", import.meta.url).pathname;

const scanDirs = ["src"];
const skipPatterns = [".test.", ".jest."];
const baselinePath = join(siteDir, "scripts", "design-tokens-baseline.json");

// Default Tailwind color palette families. Utilities that reference these
// with a numeric shade (e.g. text-red-500) sidestep the semantic tokens.
const PALETTE =
	"slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose";

// Utility prefixes that set a color.
const COLOR_PREFIX =
	"text|bg|border|fill|stroke|from|to|via|ring|outline|shadow|decoration|divide|accent|caret|placeholder";

// Spacing prefixes: padding, margin, gap, and space utilities. The
// character class covers axis/side suffixes (px, py, pt, ml, ps, ...).
const SPACING_PREFIX = "p[xytblrse]?|m[xytblrse]?|gap(?:-[xy])?|space-[xy]";

// Each rule maps to a regex and a human-readable hint. Matches are the
// raw utility strings, which double as stable baseline signatures.
const RULES = [
	{
		name: "color",
		hint: "use a semantic color token (e.g. text-content-primary, bg-surface-secondary)",
		patterns: [
			// Arbitrary hex, e.g. text-[#F87171].
			new RegExp(`\\b(?:${COLOR_PREFIX})-\\[#[0-9a-fA-F]{3,8}\\]`, "g"),
			// Arbitrary color functions, e.g. bg-[rgb(0,0,0)].
			new RegExp(
				`\\b(?:${COLOR_PREFIX})-\\[(?:rgb|rgba|hsl|hsla|oklch|oklab|lab|lch)\\(`,
				"g",
			),
			// Default palette shades, e.g. text-red-500, bg-gray-100.
			new RegExp(
				`\\b(?:${COLOR_PREFIX})-(?:${PALETTE})-(?:50|100|200|300|400|500|600|700|800|900|950)\\b`,
				"g",
			),
		],
	},
	{
		name: "font",
		hint: "use a token from the --text-* scale (e.g. text-sm, text-2xs)",
		patterns: [
			// Arbitrary numeric font size, e.g. text-[13px], text-[0.8rem].
			// text-[length:var(...)] and text-[length:inherit] are allowed
			// because the value is not a hard-coded size.
			/\btext-\[(?:length:)?\d*\.?\d+(?:px|rem|em|pt|%|vw|vh)\]/g,
		],
	},
	{
		name: "spacing",
		hint: "use the 4px spacing scale (e.g. p-2, gap-4) instead of an arbitrary length",
		patterns: [
			// Arbitrary length spacing, e.g. p-[7px], gap-[18px], mb-[0.75em].
			// var()/calc()/keyword values (auto, initial, ...) are allowed.
			new RegExp(
				`\\b(?:${SPACING_PREFIX})-\\[(?:length:)?\\d*\\.?\\d+(?:px|rem|em)\\]`,
				"g",
			),
		],
	},
];

// ---------------------------------------------------------------------------
// Detection (pure, exported for tests)
// ---------------------------------------------------------------------------

/**
 * Scan a source string and return every design-token violation as
 * `{ rule, snippet, line }`. `snippet` is the offending utility string;
 * it is stable across edits so it works as a baseline signature.
 */
export function findViolationsInText(text) {
	const violations = [];
	const lines = text.split("\n");
	for (let i = 0; i < lines.length; i++) {
		for (const rule of RULES) {
			for (const pattern of rule.patterns) {
				pattern.lastIndex = 0;
				let m;
				while ((m = pattern.exec(lines[i])) !== null) {
					violations.push({
						rule: rule.name,
						snippet: m[0],
						line: i + 1,
					});
					// Guard against zero-width matches looping forever.
					if (m.index === pattern.lastIndex) pattern.lastIndex++;
				}
			}
		}
	}
	return violations;
}

/** Stable baseline key for a violation, independent of its line number. */
export function signatureOf(violation) {
	return `${violation.rule}|${violation.snippet}`;
}

/** Look up the one-line hint for a rule name. */
export function hintFor(ruleName) {
	return RULES.find((r) => r.name === ruleName)?.hint ?? "";
}

/**
 * Collapse a file's violations into a signature→count map. Used for both
 * the baseline file and the current run so they compare directly.
 */
export function countSignatures(violations) {
	const counts = {};
	for (const v of violations) {
		const key = signatureOf(v);
		counts[key] = (counts[key] ?? 0) + 1;
	}
	return counts;
}

/**
 * Compare current per-file violations against the baseline and return the
 * occurrences that exceed the recorded counts. `currentByFile` maps a path
 * to its violation array; `baseline` maps a path to a signature→count map.
 *
 * For each signature whose current count exceeds the baseline, the newest
 * occurrences (by line) are reported so the surfaced lines point at the
 * added code rather than the grandfathered ones.
 */
export function computeNewViolations(currentByFile, baseline) {
	const results = [];
	for (const [path, violations] of Object.entries(currentByFile)) {
		const allowed = baseline[path] ?? {};
		const bySig = new Map();
		for (const v of violations) {
			const key = signatureOf(v);
			if (!bySig.has(key)) bySig.set(key, []);
			bySig.get(key).push(v);
		}
		for (const [key, occurrences] of bySig) {
			const allowedCount = allowed[key] ?? 0;
			const extra = occurrences.length - allowedCount;
			if (extra <= 0) continue;
			// Report the trailing `extra` occurrences (highest line numbers).
			const sorted = [...occurrences].sort((a, b) => a.line - b.line);
			for (const v of sorted.slice(sorted.length - extra)) {
				results.push({ path, ...v });
			}
		}
	}
	results.sort(
		(a, b) => a.path.localeCompare(b.path) || a.line - b.line,
	);
	return results;
}

// ---------------------------------------------------------------------------
// File collection
// ---------------------------------------------------------------------------

/**
 * Recursively collect .ts/.tsx files under `dir`, skipping test files.
 * Returns paths relative to `siteDir`.
 */
function collectFiles(dir) {
	const results = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const full = join(dir, entry.name);
		if (entry.isDirectory()) {
			results.push(...collectFiles(full));
		} else if (
			(entry.name.endsWith(".ts") || entry.name.endsWith(".tsx")) &&
			!skipPatterns.some((p) => entry.name.includes(p))
		) {
			results.push(relative(siteDir, full));
		}
	}
	return results;
}

/** Scan every source file and return `{ currentByFile, totalCount }`. */
function scanAll() {
	const files = scanDirs.flatMap((d) => collectFiles(join(siteDir, d)));
	const currentByFile = {};
	let totalCount = 0;
	for (const file of files) {
		const text = readFileSync(join(siteDir, file), "utf-8");
		const violations = findViolationsInText(text);
		if (violations.length > 0) {
			currentByFile[file] = violations;
			totalCount += violations.length;
		}
	}
	return { currentByFile, totalCount };
}

/** Build the baseline structure (path → signature → count) from a scan. */
function buildBaseline(currentByFile) {
	const baseline = {};
	for (const path of Object.keys(currentByFile).sort()) {
		baseline[path] = countSignatures(currentByFile[path]);
	}
	return baseline;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

if (process.argv[1] === fileURLToPath(import.meta.url)) {
	const update = process.argv.includes("--update");
	const { currentByFile, totalCount } = scanAll();

	if (update) {
		const baseline = buildBaseline(currentByFile);
		writeFileSync(baselinePath, `${JSON.stringify(baseline, null, "\t")}\n`);
		console.log(
			`✓ Wrote baseline with ${totalCount} grandfathered occurrences to ${relative(siteDir, baselinePath)}`,
		);
	} else {
		let baseline = {};
		try {
			baseline = JSON.parse(readFileSync(baselinePath, "utf-8"));
		} catch (e) {
			if (e.code !== "ENOENT") throw e;
		}

		const newViolations = computeNewViolations(currentByFile, baseline);
		if (newViolations.length === 0) {
			console.log("✓ No new design-token violations.");
		} else {
			console.error(
				`\n✗ ${newViolations.length} new design-token violation(s):\n`,
			);
			for (const v of newViolations) {
				console.error(`  ${v.path}:${v.line}  ${v.snippet}`);
				console.error(`      → ${hintFor(v.rule)}`);
			}
			console.error(
				"\nUse a design token instead. If an exception is unavoidable and " +
					"approved, run `pnpm lint:design --update` to update the baseline.\n",
			);
			process.exitCode = 1;
		}
	}
}
