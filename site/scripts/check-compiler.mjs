/**
 * React Compiler diagnostic checker.
 *
 * Runs babel-plugin-react-compiler over every .ts/.tsx file in the
 * target directories and reports functions that failed to compile or
 * were skipped. Exits with code 1 when any diagnostics are present
 * or a target directory is missing.
 *
 * Usage:  node scripts/check-compiler.mjs
 */
import { readFileSync, readdirSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { transformSync } from "@babel/core";

// Resolve the site/ directory (ESM equivalent of __dirname + "..").
const siteDir = new URL("..", import.meta.url).pathname;

// Only AgentsPage is currently opted in to React Compiler. Add new
// directories here as more pages are migrated.
const targetDirs = [
	"src/pages/AgentsPage",
];

const skipPatterns = [".test.", ".stories.", ".jest."];

// Maximum length for truncated error messages in the report.
const MAX_ERROR_LENGTH = 120;

// ---------------------------------------------------------------------------
// File collection
// ---------------------------------------------------------------------------

/**
 * Recursively collect .ts/.tsx files under `dir`, skipping test and
 * story files. Returns paths relative to `siteDir`. Sets
 * `hadCollectionErrors` and returns an empty array on ENOENT so the
 * caller and recursive calls both stay safe.
 */
function collectFiles(dir) {
	let entries;
	try {
		entries = readdirSync(dir, { withFileTypes: true });
	} catch (e) {
		if (e.code === "ENOENT") {
			console.error(`Target directory not found: ${relative(siteDir, dir)}`);
			hadCollectionErrors = true;
			return [];
		}
		throw e;
	}
	const results = [];
	for (const entry of entries) {
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

// ---------------------------------------------------------------------------
// Compilation & diagnostics
//
// We use transformSync deliberately. The React Compiler plugin is
// CPU-bound (parse-only takes ~2s vs ~19s with the compiler over all
// of site/src), so transformAsync + Promise.all gives no speedup —
// Node still runs all transforms on a single thread. Benchmarked
// sync, async-sequential, and async-parallel: all land within noise
// of each other. The sync API keeps the code simple.
// ---------------------------------------------------------------------------

/**
 * Shorten a compiler diagnostic message to its first sentence, stripping
 * the leading "Error: " prefix and any trailing URL references so the
 * one-line report stays readable.
 *
 * Example:
 *   "Error: Ref values are not allowed. Use ref types instead (https://…)."
 *   → "Ref values are not allowed"
 */
export function shortenMessage(msg) {
	const str = typeof msg === "string" ? msg : String(msg);
	return str
		.replace(/^Error: /, "")
		.split(/\.\s/)[0]
		.split("(http")[0]
		.replace(/\.\s*$/, "")
		.trim();
}

/**
 * Remove diagnostics that share the same line + message. The compiler
 * can emit duplicate events for the same function when it retries
 * compilation, so we deduplicate before reporting.
 */
export function deduplicateDiagnostics(diagnostics) {
	const seen = new Set();
	return diagnostics.filter((d) => {
		const key = `${d.line}:${d.short}`;
		if (seen.has(key)) return false;
		seen.add(key);
		return true;
	});
}

/**
 * Run the React Compiler over a single file and return the number of
 * successfully compiled functions plus any diagnostics. Transform
 * errors are caught and returned as a diagnostic with line 0 rather
 * than thrown, so the caller always gets a result.
 */
function compileFile(file) {
	const isTSX = file.endsWith(".tsx");
	const diagnostics = [];

	try {
		const code = readFileSync(join(siteDir, file), "utf-8");
		const result = transformSync(code, {
			plugins: [
				["@babel/plugin-syntax-typescript", { isTSX }],
				["babel-plugin-react-compiler", {
					logger: {
						logEvent(_filename, event) {
							if (event.kind === "CompileError" || event.kind === "CompileSkip") {
								const msg = event.detail || event.reason || "(unknown)";
								diagnostics.push({
									line: event.fnLoc?.start?.line ?? 0,
									short: shortenMessage(msg),
								});
							}
						},
					},
				}],
			],
			filename: file,
			// Skip config-file resolution. No babel.config.js exists in the
			// repo, so the search is wasted I/O on every file.
			configFile: false,
			babelrc: false,
		});

		// The compiler inserts `const $ = _c(N)` at the top of every
		// function it successfully compiles, where N is the number of
		// memoization slots. Counting these tells us how many functions
		// were compiled in this file.
		const compiledCount = result?.code?.match(/const \$ = _c\(\d+\)/g)?.length ?? 0;

		return {
			compiled: compiledCount,
			diagnostics: deduplicateDiagnostics(diagnostics),
		};
	} catch (e) {
		return {
			compiled: 0,
			diagnostics: [{
				line: 0,
				// Truncate to keep the one-line report readable.
				short: `Transform error: ${(e instanceof Error ? e.message : String(e)).substring(0, MAX_ERROR_LENGTH)}`,
			}],
		};
	}
}

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

/**
 * Derive a short display path by stripping the first matching target
 * dir prefix so the output stays compact.
 */
export function shortPath(file, dirs = targetDirs) {
	for (const dir of dirs) {
		const prefix = `${dir}/`;
		if (file.startsWith(prefix)) {
			return file.slice(prefix.length);
		}
	}
	return file;
}

/** Print a summary of compilation results and per-file diagnostics. */
function printReport(failures, totalCompiled, fileCount, hadErrors) {
	console.log(`\nTotal: ${totalCompiled} functions compiled across ${fileCount} files`);
	console.log(`Files with diagnostics: ${failures.length}\n`);

	for (const f of failures) {
		console.log(`✗ ${shortPath(f.file)} (${f.compiled} compiled)`);
		for (const d of f.diagnostics) {
			console.log(`    line ${d.line}: ${d.short}`);
		}
	}

	if (failures.length === 0 && !hadErrors) {
		console.log("✓ All files compile cleanly.");
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

// Only run the main block when executed directly, not when imported
// by tests for the exported pure functions.
if (process.argv[1] === fileURLToPath(import.meta.url)) {
	let hadCollectionErrors = false;

	const files = targetDirs.flatMap((d) => collectFiles(join(siteDir, d)));

let totalCompiled = 0;
const failures = [];

for (const file of files) {
	const { compiled, diagnostics } = compileFile(file);
	totalCompiled += compiled;
	if (diagnostics.length > 0) {
		failures.push({ file, compiled, diagnostics });
	}
}

printReport(failures, totalCompiled, files.length, hadCollectionErrors);

if (failures.length > 0 || hadCollectionErrors) {
	process.exitCode = 1;
}
}
