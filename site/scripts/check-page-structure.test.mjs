import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it, vi } from "vitest";
import {
	BAD_NAME,
	KNOWN_EXCEPTIONS,
	NOT_A_DIRECTORY,
	PAGE_DIR_PATTERN,
	STALE_EXCEPTION_COMPLIANT,
	STALE_EXCEPTION_MISSING,
	checkPageEntries,
	formatReport,
	readPageEntries,
	runCli,
	violationFor,
} from "./check-page-structure.mjs";

const dir = (name) => ({ name, isDirectory: true });
const file = (name) => ({ name, isDirectory: false });

// Vitest runs with site/ as its root, so resolve from the working
// directory. import.meta.url is not a file URL inside the jsdom
// environment.
const pagesDir = path.resolve(process.cwd(), "src/pages");

const withTmpDir = (fn) => {
	const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "check-page-structure-"));
	try {
		return fn(tmpDir);
	} finally {
		fs.rmSync(tmpDir, { recursive: true, force: true });
	}
};

describe("PAGE_DIR_PATTERN", () => {
	it.each([
		"WorkspacesPage",
		"AIBridgePage",
		"OAuth2Page",
		"LoginOAuthDevicePage",
	])("accepts %s", (name) => {
		expect(PAGE_DIR_PATTERN.test(name)).toBe(true);
	});

	it.each([
		"workspacesPage", // lowercase first letter
		"Workspaces", // missing suffix
		"WorkspacesPages", // suffix must be final
		"Workspaces_Page", // underscore
		"workspaces-page", // kebab-case
		"Page", // suffix with no name
		"", // empty
	])("rejects %s", (name) => {
		expect(PAGE_DIR_PATTERN.test(name)).toBe(false);
	});
});

describe("violationFor", () => {
	it("returns null for a compliant directory", () => {
		expect(violationFor(dir("WorkspacesPage"))).toBeNull();
	});

	it("rejects a non-directory even when the name is compliant", () => {
		expect(violationFor(file("WorkspacesPage"))).toBe(NOT_A_DIRECTORY);
	});

	it("rejects a directory with a non-compliant name", () => {
		expect(violationFor(dir("TemplateBuilder"))).toBe(BAD_NAME);
	});
});

describe("checkPageEntries", () => {
	it("passes when every entry is a compliant directory", () => {
		expect(
			checkPageEntries([dir("AuditPage"), dir("WorkspacesPage")], []),
		).toEqual({ violations: [], staleExceptions: [] });
	});

	it("flags a new directory that does not end with Page", () => {
		const { violations } = checkPageEntries(
			[dir("AuditPage"), dir("TemplateBuilder")],
			[],
		);
		expect(violations).toEqual([{ name: "TemplateBuilder", reason: BAD_NAME }]);
	});

	it("flags a loose file at the top level", () => {
		const { violations } = checkPageEntries([file("index.ts")], []);
		expect(violations).toEqual([{ name: "index.ts", reason: NOT_A_DIRECTORY }]);
	});

	it("forgives a listed exception", () => {
		expect(
			checkPageEntries([dir("AuditPage"), dir("TemplateBuilder")], [
				"TemplateBuilder",
			]),
		).toEqual({ violations: [], staleExceptions: [] });
	});

	it("still flags other invalid names when an exception is listed", () => {
		const { violations } = checkPageEntries(
			[dir("TemplateBuilder"), dir("workspaces")],
			["TemplateBuilder"],
		);
		expect(violations).toEqual([{ name: "workspaces", reason: BAD_NAME }]);
	});

	it("reports an exception whose directory no longer exists", () => {
		const { staleExceptions } = checkPageEntries([dir("AuditPage")], [
			"TemplateBuilder",
		]);
		expect(staleExceptions).toEqual([
			{ name: "TemplateBuilder", reason: STALE_EXCEPTION_MISSING },
		]);
	});

	it("reports an exception that now satisfies the rule", () => {
		const { staleExceptions } = checkPageEntries([dir("TemplateBuilderPage")], [
			"TemplateBuilderPage",
		]);
		expect(staleExceptions).toEqual([
			{ name: "TemplateBuilderPage", reason: STALE_EXCEPTION_COMPLIANT },
		]);
	});

	it("rejects invalid names by default", () => {
		expect(checkPageEntries([dir("TemplateBuilder")]).violations).toEqual([
			{ name: "TemplateBuilder", reason: BAD_NAME },
		]);
	});

	it("reports violations in directory order", () => {
		const { violations } = checkPageEntries(
			[dir("aaa"), dir("AuditPage"), dir("zzz")],
			[],
		);
		expect(violations.map((v) => v.name)).toEqual(["aaa", "zzz"]);
	});
});

describe("formatReport", () => {
	it("renders a success line when there is nothing to report", () => {
		const report = formatReport(
			{ violations: [], staleExceptions: [] },
			"/pages",
		);
		expect(report).toContain("✓ /pages");
		expect(report).not.toContain("✗");
	});

	it("lists violations with their reason and a fix hint", () => {
		const report = formatReport(
			{ violations: [{ name: "workspaces", reason: BAD_NAME }], staleExceptions: [] },
			"/pages",
		);
		expect(report).toContain("✗ workspaces: " + BAD_NAME);
		expect(report).toContain('"WorkspacesPage"');
	});

	it("lists stale exceptions and points at the exception list", () => {
		const report = formatReport(
			{
				violations: [],
				staleExceptions: [
					{ name: "TemplateBuilder", reason: STALE_EXCEPTION_MISSING },
				],
			},
			"/pages",
		);
		expect(report).toContain("Stale entries in KNOWN_EXCEPTIONS");
		expect(report).toContain("✗ TemplateBuilder: " + STALE_EXCEPTION_MISSING);
		expect(report).toContain("scripts/check-page-structure.mjs");
	});
});

describe("readPageEntries", () => {
	it("returns direct children sorted by name with directory flags", () => {
		withTmpDir((tmpDir) => {
			fs.mkdirSync(path.join(tmpDir, "WorkspacesPage"));
			fs.mkdirSync(path.join(tmpDir, "AuditPage"));
			fs.writeFileSync(path.join(tmpDir, "index.ts"), "export {};\n");
			// Nested content must not surface as a top-level entry.
			fs.writeFileSync(
				path.join(tmpDir, "AuditPage", "AuditPage.tsx"),
				"export default null;\n",
			);

			expect(readPageEntries(tmpDir)).toEqual([
				{ name: "AuditPage", isDirectory: true },
				{ name: "WorkspacesPage", isDirectory: true },
				{ name: "index.ts", isDirectory: false },
			]);
		});
	});
});

describe("runCli", () => {
	const run = (argv) => {
		const logSpy = vi.spyOn(console, "log").mockImplementation(() => {});
		try {
			const code = runCli(argv);
			return { code, output: logSpy.mock.calls.map((c) => c.join(" ")).join("\n") };
		} finally {
			logSpy.mockRestore();
		}
	};

	// The CLI uses the shipped exception list, so a fixture directory has
	// to contain those directories or they are reported as stale.
	const seedExceptions = (tmpDir) => {
		for (const name of KNOWN_EXCEPTIONS) {
			fs.mkdirSync(path.join(tmpDir, name));
		}
	};

	it("exits 0 for a compliant directory", () => {
		withTmpDir((tmpDir) => {
			seedExceptions(tmpDir);
			fs.mkdirSync(path.join(tmpDir, "WorkspacesPage"));
			const { code, output } = run([`--dir=${tmpDir}`]);
			expect(code).toBe(0);
			expect(output).toContain("✓");
		});
	});

	it("exits 1 and names the offender for an invalid directory", () => {
		withTmpDir((tmpDir) => {
			seedExceptions(tmpDir);
			fs.mkdirSync(path.join(tmpDir, "workspaces"));
			const { code, output } = run([`--dir=${tmpDir}`]);
			expect(code).toBe(1);
			expect(output).toContain("✗ workspaces");
		});
	});

	it("defaults to site/src/pages and passes on the real tree", () => {
		const { code } = run([]);
		expect(code).toBe(0);
	});
});

describe("site/src/pages", () => {
	it("has no invalid top-level entries and no stale exceptions", () => {
		const result = checkPageEntries(readPageEntries(pagesDir));
		expect(formatReport(result, "src/pages")).toContain("✓");
		expect(result).toEqual({ violations: [], staleExceptions: [] });
	});

	it("requires every top-level directory to follow the naming rule", () => {
		expect(KNOWN_EXCEPTIONS).toEqual([]);
	});
});
