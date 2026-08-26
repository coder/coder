/**
 * Top-level page directory structure checker.
 *
 * Every entry directly under src/pages must be a directory named in
 * PascalCase and suffixed with "Page" (for example "WorkspacesPage").
 * Directories that predate this rule are listed in KNOWN_EXCEPTIONS so
 * the check passes today while still rejecting any new invalid name.
 *
 * Exceptions are also checked in reverse: an entry listed here that no
 * longer exists, or that now satisfies the rule, is reported as stale so
 * the list shrinks to nothing as the migration lands.
 *
 * Usage:  node scripts/check-page-structure.mjs [--dir=<path>]
 *
 * Exits with code 1 when any violation or stale exception is found.
 */

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";

// PascalCase, suffixed with "Page", at least one character before the
// suffix. Digits are allowed after the first character so names like
// "OAuth2Page" pass. "Page" on its own does not match.
export const PAGE_DIR_PATTERN = /^[A-Z][A-Za-z0-9]*Page$/;

// Directories that violate the rule but predate it. Do not add entries.
// Remove each one as its directory is migrated to a compliant name.
export const KNOWN_EXCEPTIONS = ["TemplateBuilder"];

export const NOT_A_DIRECTORY = "not a directory";
export const BAD_NAME = 'name must be PascalCase and end with "Page"';
export const STALE_EXCEPTION_MISSING = "no longer exists";
export const STALE_EXCEPTION_COMPLIANT = "now satisfies the naming rule";

/**
 * Read the direct children of `dir` as `{ name, isDirectory }` records,
 * sorted by name so reports are stable across filesystems.
 */
export function readPageEntries(dir) {
	return fs
		.readdirSync(dir, { withFileTypes: true })
		.map((e) => ({ name: e.name, isDirectory: e.isDirectory() }))
		.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0));
}

/**
 * Classify a single entry. Returns the violation reason, or null when
 * the entry satisfies the rule.
 */
export function violationFor(entry) {
	if (!entry.isDirectory) return NOT_A_DIRECTORY;
	if (!PAGE_DIR_PATTERN.test(entry.name)) return BAD_NAME;
	return null;
}

/**
 * Check every entry against the naming rule and the exception list.
 *
 * Returns `{ violations, staleExceptions }`, both arrays of
 * `{ name, reason }`. Violations are entries the rule rejects that are
 * not excepted; stale exceptions are excepted names the rule no longer
 * needs to forgive.
 */
export function checkPageEntries(entries, exceptions = KNOWN_EXCEPTIONS) {
	const excepted = new Set(exceptions);
	const violations = [];

	for (const entry of entries) {
		const reason = violationFor(entry);
		if (reason === null || excepted.has(entry.name)) continue;
		violations.push({ name: entry.name, reason });
	}

	const byName = new Map(entries.map((e) => [e.name, e]));
	const staleExceptions = [];
	for (const name of exceptions) {
		const entry = byName.get(name);
		if (entry === undefined) {
			staleExceptions.push({ name, reason: STALE_EXCEPTION_MISSING });
		} else if (violationFor(entry) === null) {
			staleExceptions.push({ name, reason: STALE_EXCEPTION_COMPLIANT });
		}
	}

	return { violations, staleExceptions };
}

/** Render a human-readable report for a checkPageEntries result. */
export function formatReport({ violations, staleExceptions }, dir) {
	if (violations.length === 0 && staleExceptions.length === 0) {
		return `✓ ${dir}: all top-level entries follow the page naming rule.`;
	}

	const lines = [];
	if (violations.length > 0) {
		lines.push(`Invalid top-level entries in ${dir}:`);
		for (const v of violations) {
			lines.push(`  ✗ ${v.name}: ${v.reason}`);
		}
		lines.push("");
		lines.push(
			'Name each page directory in PascalCase with a "Page" suffix, for example "WorkspacesPage".',
		);
	}
	if (staleExceptions.length > 0) {
		if (lines.length > 0) lines.push("");
		lines.push("Stale entries in KNOWN_EXCEPTIONS:");
		for (const s of staleExceptions) {
			lines.push(`  ✗ ${s.name}: ${s.reason}`);
		}
		lines.push("");
		lines.push(
			"Remove them from KNOWN_EXCEPTIONS in scripts/check-page-structure.mjs.",
		);
	}
	return lines.join("\n");
}

export function runCli(argv) {
	const { values: args } = parseArgs({
		args: argv,
		options: { dir: { type: "string" } },
	});
	const dir =
		args.dir ??
		path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "src", "pages");

	const result = checkPageEntries(readPageEntries(dir));
	console.log(formatReport(result, dir));
	return result.violations.length === 0 && result.staleExceptions.length === 0
		? 0
		: 1;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
	process.exitCode = runCli(process.argv.slice(2));
}
