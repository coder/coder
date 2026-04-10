import { promises as fs } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

type FsGlob = (
	pattern: string,
	options?: {
		cwd?: string;
	},
) => AsyncIterable<string>;

const glob = (fs as typeof fs & { glob: FsGlob }).glob;
const eagerMonacoImportPattern =
	/^\s*import\s+(?!type\b)[^;]*\bfrom\s*["']monaco-editor["']\s*;?/m;

describe("monaco loading", () => {
	it("does not use eager monaco-editor imports in production source files", async () => {
		const srcDir = path.resolve(process.cwd(), "src");
		const sourceFiles: string[] = [];

		for await (const filePath of glob("**/*.{ts,tsx}", { cwd: srcDir })) {
			sourceFiles.push(filePath);
		}

		const violations: string[] = [];
		const filesToCheck = sourceFiles
			.map((filePath) => filePath.replaceAll(path.sep, "/"))
			.filter((filePath) => !filePath.includes("node_modules"))
			.filter((filePath) => !filePath.includes(".stories."))
			.filter((filePath) => !filePath.includes(".test."))
			.filter((filePath) => filePath !== "utils/monaco.ts")
			.sort();

		for (const relativePath of filesToCheck) {
			const absolutePath = path.join(srcDir, relativePath);
			const fileContents = await fs.readFile(absolutePath, "utf8");

			if (eagerMonacoImportPattern.test(fileContents)) {
				violations.push(
					`Found eager monaco-editor import in src/${relativePath}. Use ensureMonacoIsLoaded() from #/utils/monaco instead.`,
				);
			}
		}

		expect(violations.join("\n")).toBe("");
	});
});
