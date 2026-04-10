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

// Patterns that identify a function/closure value on the RHS of an
// assignment. Primitives (strings, numbers, booleans) are fine without
// memoization because `!==` compares them by value. Only reference types
// (closures, objects, arrays) cause problems.
const CLOSURE_RHS = /^\s*(?:const|let)\s+(\w+)\s*=\s*(?:async\s+)?(?:\([^)]*\)\s*=>|\w+\s*=>|function\s*\()/;

// Matches a `$[N] !== name` fragment inside an `if (...)` guard.
const DEP_CHECK = /\$\[\d+\]\s*!==\s*(\w+)/g;

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
// of site/src), so transformAsync + Promise.all gives no speedup
// because Node still runs all transforms on a single thread. Benchmarked
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
			code: result?.code ?? "",
			diagnostics: deduplicateDiagnostics(diagnostics),
		};
	} catch (e) {
		return {
			compiled: 0,
			code: "",
			diagnostics: [{
				line: 0,
				// Truncate to keep the one-line report readable.
				short: `Transform error: ${(e instanceof Error ? e.message : String(e)).substring(0, MAX_ERROR_LENGTH)}`,
			}],
		};
	}
}

// ---------------------------------------------------------------------------
// Scope-pruning detection
//
// The compiler's flattenScopesWithHooksOrUse pass silently drops
// memoization scopes that span across hook calls. A closure whose
// scope is pruned appears as a bare `const name = (...) =>` with
// no `$[N]` guard, yet it may still be listed as a dependency in a
// downstream JSX memoization block (`$[N] !== name`). That means
// the JSX cache check fails every render because `name` is a new
// function reference each time.
//
// findUnmemoizedClosureDeps detects this pattern in compiled output:
// 1. Collect every name that appears in a `$[N] !== name` dep check.
// 2. For each, check if the name is assigned a function value
//    (arrow or function expression) outside any `$[N]` guard.
// 3. If so, the closure is unmemoized but used as a reactive dep,
//    which defeats the downstream memoization.
// ---------------------------------------------------------------------------

/**
 * Scan compiled output for closures that appear as dependencies in
 * memoization guards but are not themselves memoized. Returns an
 * array of `{ name, line }` objects for each finding.
 */
export function findUnmemoizedClosureDeps(code) {
	if (!code) return [];

	const lines = code.split("\n");

	// Pass 1: collect every name used in a $[N] !== name dep check.
	const depNames = new Set();
	for (const line of lines) {
		for (const m of line.matchAll(DEP_CHECK)) {
			depNames.add(m[1]);
		}
	}
	if (depNames.size === 0) return [];

	// Pass 2: find closure definitions that are directly assigned a
	// function value (not assigned from a temp like `const x = t1`).
	// A memoized closure uses the temp pattern:
	//   if ($[N] !== dep) { t1 = () => {...}; } else { t1 = $[N]; }
	//   const name = t1;
	// An unmemoized closure is assigned the function directly:
	//   const name = () => {...};
	const findings = [];
	for (let i = 0; i < lines.length; i++) {
		const match = lines[i].match(CLOSURE_RHS);
		if (!match) continue;

		const name = match[1];
		if (!depNames.has(name)) continue;

		// Compiler temporaries are named t0, t1, ... tN. If the
		// variable name matches that pattern it's an intermediate,
		// not a user-visible declaration.
		if (/^t\d+$/.test(name)) continue;

		findings.push({ name, line: i + 1 });
	}

	return findings;
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

// Tracks whether collectFiles encountered a missing directory.
// Module-scoped so the function can set it and the main block can
// read it after collection finishes.
let hadCollectionErrors = false;

// Only run the main block when executed directly, not when imported
// by tests for the exported pure functions.
if (process.argv[1] === fileURLToPath(import.meta.url)) {

	const files = targetDirs.flatMap((d) => collectFiles(join(siteDir, d)));

	let totalCompiled = 0;
	const failures = [];

	const scopePruned = [];

	for (const file of files) {
		const { compiled, code, diagnostics } = compileFile(file);
		totalCompiled += compiled;
		if (diagnostics.length > 0) {
			failures.push({ file, compiled, diagnostics });
		}
		const pruned = findUnmemoizedClosureDeps(code);
		if (pruned.length > 0) {
			scopePruned.push({ file, closures: pruned });
		}
	}

	printReport(failures, totalCompiled, files.length, hadCollectionErrors);

	if (scopePruned.length > 0) {
		console.log("\nUnmemoized closures used as reactive dependencies:");
		console.log("(Move these after all hook calls to restore memoization)\n");
		for (const { file, closures } of scopePruned) {
			for (const c of closures) {
				console.log(`  ✗ ${shortPath(file)}: ${c.name}`);
			}
		}
	}

	if (failures.length > 0 || hadCollectionErrors || scopePruned.length > 0) {
		process.exitCode = 1;
	}
}
