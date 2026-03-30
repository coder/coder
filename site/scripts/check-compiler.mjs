/**
 * React Compiler diagnostic checker.
 *
 * Runs babel-plugin-react-compiler over every .ts/.tsx file in the
 * target directories and reports functions that failed to compile or
 * were skipped. Exits with code 1 when any diagnostics are present.
 *
 * Usage:  node scripts/check-compiler.mjs
 */
import { readFileSync, readdirSync } from "node:fs";
import { join, relative } from "node:path";
import { transformSync } from "@babel/core";

const siteDir = new URL("..", import.meta.url).pathname;

const targetDirs = [
	"src/pages/AgentsPage",
];

const skipPatterns = [".test.", ".stories.", ".jest."];

// ---------------------------------------------------------------------------
// File collection
// ---------------------------------------------------------------------------

/**
 * Recursively collect .ts/.tsx files under `dir`, skipping test and
 * story files. Returns paths relative to `siteDir`.
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

// ---------------------------------------------------------------------------
// Compilation & diagnostics
// ---------------------------------------------------------------------------

/**
 * Shorten a compiler diagnostic message to its first sentence, stripping
 * the leading "Error: " prefix and any trailing URL references so the
 * one-line report stays readable.
 */
function shortenMessage(msg) {
	if (typeof msg !== "string") {
		return String(msg);
	}
	return msg
		.replace(/^Error: /, "")
		.split(".")[0]
		.split("(http")[0]
		.trim();
}

/** Remove diagnostics that share the same line + message. */
function deduplicateDiagnostics(diagnostics) {
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
 * successfully compiled functions plus any diagnostics.
 */
function compileFile(file) {
	const code = readFileSync(join(siteDir, file), "utf-8");
	const isTSX = file.endsWith(".tsx");
	const diagnostics = [];

	try {
		const result = transformSync(code, {
			plugins: [
				["@babel/plugin-syntax-typescript", { isTSX }],
				["babel-plugin-react-compiler", {
					logger: {
						logEvent(_filename, event) {
							if (event.kind === "CompileError" || event.kind === "CompileSkip") {
								const msg = event.detail || event.reason || "";
								diagnostics.push({
									line: event.fnLoc?.start?.line,
									short: shortenMessage(msg),
								});
							}
						},
					},
				}],
			],
			filename: file,
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
				short: `Transform error: ${String(e.message).substring(0, 120)}`,
			}],
		};
	}
}

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

/**
 * Derive a short display path by stripping the longest matching target
 * dir prefix so the output stays compact.
 */
function shortPath(file) {
	for (const dir of targetDirs) {
		const prefix = `${dir}/`;
		if (file.startsWith(prefix)) {
			return file.slice(prefix.length);
		}
	}
	return file;
}

function printReport(failures, totalCompiled, fileCount) {
	console.log(`\nTotal: ${totalCompiled} functions compiled across ${fileCount} files`);
	console.log(`Files with diagnostics: ${failures.length}\n`);

	for (const f of failures) {
		console.log(`✗ ${shortPath(f.file)} (${f.compiled} compiled)`);
		for (const d of f.diagnostics) {
			console.log(`    line ${d.line}: ${d.short}`);
		}
	}

	if (failures.length === 0) {
		console.log("✓ All files compile cleanly.");
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

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

printReport(failures, totalCompiled, files.length);

if (failures.length > 0) {
	process.exitCode = 1;
}
