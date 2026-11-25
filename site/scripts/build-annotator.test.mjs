import { execFile } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { Script } from "node:vm";
import { expect, it } from "vitest";

const exec = promisify(execFile);
const siteDir = path.resolve(import.meta.dirname, "..");

it("builds a classic annotator script without deleting the dashboard", async () => {
	const outDir = await mkdtemp(path.join(tmpdir(), "coder-annotator-build-"));
	try {
		const index = path.join(outDir, "index.html");
		await writeFile(index, "existing dashboard");
		await exec("pnpm", ["build:annotator", "--outDir", outDir], {
			cwd: siteDir,
			timeout: 30_000,
		});

		const script = await readFile(path.join(outDir, "annotator.js"), "utf8");
		expect(script.length).toBeGreaterThan(0);
		// A classic script cannot contain the dashboard's ESM imports or exports.
		expect(() => new Script(script)).not.toThrow();
		expect(await readFile(index, "utf8")).toBe("existing dashboard");
	} finally {
		await rm(outDir, { recursive: true, force: true });
	}
}, 30_000);
