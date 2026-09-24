import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { afterAll, describe, expect, it } from "vitest";

const siteDir = path.resolve(__dirname, "..");
const oxlint = path.join(siteDir, "node_modules/oxlint/bin/oxlint");
const fixtureDir = mkdtempSync(path.join(siteDir, ".design-lint-test-"));

afterAll(() => {
	rmSync(fixtureDir, { recursive: true, force: true });
});

function lint(name, source, advisory = true) {
	const dir = path.join(fixtureDir, name);
	mkdirSync(dir);
	writeFileSync(path.join(dir, "view.tsx"), source);
	writeFileSync(path.join(dir, "typesGenerated.ts"), "export interface Generated {}");
	return spawnSync(
		process.execPath,
		[
			oxlint,
			"--config",
			advisory ? ".oxlintrc.design-system.jsonc" : ".oxlintrc.jsonc",
			...(advisory ? [] : ["--deny-warnings"]),
			"--format=json",
			dir,
		],
		{ cwd: siteDir, encoding: "utf8", timeout: 10_000 },
	);
}

describe("design-system lint", () => {
	it.each([
		[
			"semantic-token",
			'export const view = <div className="text-content-primary" />;',
		],
		[
			"other-rules-disabled",
			'export const view = <div className="p-[13px]" style={{ width: 13 }} />;',
		],
	])("accepts %s and excludes generated files", (name, source) => {
		const result = lint(name, source);
		expect(result.status, result.stderr).toBe(0);
		expect(result.stderr).toBe("");
		expect(JSON.parse(result.stdout).diagnostics).toEqual([]);
	});

	it.each([
		[
			"raw-palette",
			'import { cn } from "cn"; export const view = <div className={cn("text-red-500")} />;',
		],
		[
			"undeclared-token",
			'export const view = <div className="bg-surface-not-declared" />;',
		],
	])("reports %s without failing", (name, source) => {
		const result = lint(name, source);
		expect(result.status, result.stderr).toBe(0);
		expect(JSON.parse(result.stdout).diagnostics).toEqual([
			expect.objectContaining({
				code: "shadcn(no-raw-colors)",
				severity: "warning",
			}),
		]);
	});

	it("keeps raw-color warnings out of blocking lint", () => {
		const result = lint(
			"blocking-lint",
			'export const view = <div className="text-red-500" />;',
			false,
		);
		expect(result.status, result.stderr).toBe(0);
		expect(JSON.parse(result.stdout).diagnostics).toEqual([]);
	});

	it("still fails on source errors", () => {
		const result = lint("syntax-error", "export const view = <div");
		expect(result.status).toBe(1);
		expect(JSON.parse(result.stdout).diagnostics).toEqual(
			expect.arrayContaining([expect.objectContaining({ severity: "error" })]),
		);
	});
});
