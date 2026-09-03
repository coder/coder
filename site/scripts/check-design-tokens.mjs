/**
 * Design token linter. Enforces three rules on Tailwind utilities so the
 * UI stays consistent with the tokens in src/index.css:
 *
 *   color   - semantic tokens only (text-content-primary, bg-surface-*).
 *             Rejects raw hex/rgb/hsl and the default palette (text-red-500).
 *   font    - font sizes from the --text-* scale. Rejects text-[13px].
 *   spacing - padding/margin/gap/space on the 4px scale. Rejects p-[7px].
 *             var()/calc()/keyword arbitrary values are allowed.
 *
 * Existing occurrences are grandfathered in design-tokens-baseline.json;
 * only new ones fail. Run with --update to regenerate the baseline.
 */
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const siteDir = new URL("..", import.meta.url).pathname;
const scanDirs = ["src"];
const skipPatterns = [".test.", ".jest."];
const baselinePath = join(siteDir, "scripts", "design-tokens-baseline.json");

const PALETTE =
	"slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose";
const COLOR =
	"text|bg|border|fill|stroke|from|to|via|ring|outline|shadow|decoration|divide|accent|caret|placeholder";
const SPACING = "p[xytblrse]?|m[xytblrse]?|gap(?:-[xy])?|space-[xy]";

const RULES = [
	{
		name: "color",
		hint: "use a semantic color token (e.g. text-content-primary, bg-surface-secondary)",
		patterns: [
			new RegExp(`\\b(?:${COLOR})-\\[#[0-9a-fA-F]{3,8}\\]`, "g"),
			new RegExp(`\\b(?:${COLOR})-\\[(?:rgb|rgba|hsl|hsla|oklch|oklab|lab|lch)\\(`, "g"),
			new RegExp(`\\b(?:${COLOR})-(?:${PALETTE})-(?:50|100|200|300|400|500|600|700|800|900|950)\\b`, "g"),
		],
	},
	{
		name: "font",
		hint: "use a token from the --text-* scale (e.g. text-sm, text-2xs)",
		// text-[length:var(...)] / text-[length:inherit] stay allowed (not a fixed size).
		patterns: [/\btext-\[(?:length:)?\d*\.?\d+(?:px|rem|em|pt|%|vw|vh)\]/g],
	},
	{
		name: "spacing",
		hint: "use the 4px spacing scale (e.g. p-2, gap-4) instead of an arbitrary length",
		patterns: [new RegExp(`\\b(?:${SPACING})-\\[(?:length:)?\\d*\\.?\\d+(?:px|rem|em)\\]`, "g")],
	},
];

/** Return every violation in `text` as `{ rule, snippet, line }`. */
export function findViolationsInText(text) {
	const violations = [];
	const lines = text.split("\n");
	for (let i = 0; i < lines.length; i++) {
		for (const rule of RULES) {
			for (const pattern of rule.patterns) {
				pattern.lastIndex = 0;
				let m;
				while ((m = pattern.exec(lines[i])) !== null) {
					violations.push({ rule: rule.name, snippet: m[0], line: i + 1 });
					if (m.index === pattern.lastIndex) pattern.lastIndex++;
				}
			}
		}
	}
	return violations;
}

/** Stable baseline key, independent of line number. */
export function signatureOf(v) {
	return `${v.rule}|${v.snippet}`;
}

export function hintFor(ruleName) {
	return RULES.find((r) => r.name === ruleName)?.hint ?? "";
}

/** Collapse violations into a signature→count map. */
export function countSignatures(violations) {
	const counts = {};
	for (const v of violations) {
		const key = signatureOf(v);
		counts[key] = (counts[key] ?? 0) + 1;
	}
	return counts;
}

/**
 * Return the occurrences in `currentByFile` (path → violation[]) that exceed
 * the per-signature counts in `baseline` (path → signature → count). The
 * highest-line occurrences are surfaced so reported lines point at new code.
 */
export function computeNewViolations(currentByFile, baseline) {
	const results = [];
	for (const [path, violations] of Object.entries(currentByFile)) {
		const allowed = baseline[path] ?? {};
		const bySig = new Map();
		for (const v of violations) {
			const key = signatureOf(v);
			(bySig.get(key) ?? bySig.set(key, []).get(key)).push(v);
		}
		for (const [key, occ] of bySig) {
			const extra = occ.length - (allowed[key] ?? 0);
			if (extra <= 0) continue;
			const sorted = [...occ].sort((a, b) => a.line - b.line);
			for (const v of sorted.slice(-extra)) results.push({ path, ...v });
		}
	}
	return results.sort((a, b) => a.path.localeCompare(b.path) || a.line - b.line);
}

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

function scanAll() {
	const files = scanDirs.flatMap((d) => collectFiles(join(siteDir, d)));
	const currentByFile = {};
	let totalCount = 0;
	for (const file of files) {
		const violations = findViolationsInText(readFileSync(join(siteDir, file), "utf-8"));
		if (violations.length > 0) {
			currentByFile[file] = violations;
			totalCount += violations.length;
		}
	}
	return { currentByFile, totalCount };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
	const { currentByFile, totalCount } = scanAll();

	if (process.argv.includes("--update")) {
		const baseline = {};
		for (const path of Object.keys(currentByFile).sort()) {
			baseline[path] = countSignatures(currentByFile[path]);
		}
		writeFileSync(baselinePath, `${JSON.stringify(baseline, null, "\t")}\n`);
		console.log(`✓ Wrote baseline with ${totalCount} grandfathered occurrences.`);
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
			console.error(`\n✗ ${newViolations.length} new design-token violation(s):\n`);
			for (const v of newViolations) {
				console.error(`  ${v.path}:${v.line}  ${v.snippet}`);
				console.error(`      → ${hintFor(v.rule)}`);
			}
			console.error(
				"\nUse a design token instead. For an approved exception, run " +
					"`pnpm lint:design --update` to update the baseline.\n",
			);
			process.exitCode = 1;
		}
	}
}
