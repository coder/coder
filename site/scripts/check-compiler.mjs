import { readFileSync, readdirSync } from "node:fs";
import { join, relative } from "node:path";
import { transformSync } from "@babel/core";

const siteDir = new URL("..", import.meta.url).pathname;

const targetDirs = [
	"src/pages/AgentsPage",
	"src/components/ai-elements",
];

const skipPatterns = [".test.", ".stories.", ".jest."];

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

const files = targetDirs.flatMap((d) => collectFiles(join(siteDir, d)));

let totalCompiled = 0;
const failures = [];

for (const file of files) {
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
								const short = typeof msg === "string"
									? msg.replace(/^Error: /, "").split(".")[0].split("(http")[0].trim()
									: String(msg);
								diagnostics.push({ line: event.fnLoc?.start?.line, short });
							}
						},
					},
				}],
			],
			filename: file,
		});

		const slots = result.code.match(/const \$ = _c\(\d+\)/g) || [];
		totalCompiled += slots.length;

		if (diagnostics.length) {
			const seen = new Set();
			const unique = diagnostics.filter((d) => {
				const key = `${d.line}:${d.short}`;
				if (seen.has(key)) return false;
				seen.add(key);
				return true;
			});
			failures.push({ file, compiled: slots.length, diagnostics: unique });
		}
	} catch (e) {
		failures.push({
			file, compiled: 0,
			diagnostics: [{ line: 0, short: `Transform error: ${String(e.message).substring(0, 120)}` }],
		});
	}
}

console.log(`\nTotal: ${totalCompiled} functions compiled across ${files.length} files`);
console.log(`Files with diagnostics: ${failures.length}\n`);

for (const f of failures) {
	const short = f.file.replace("src/pages/AgentsPage/", "").replace("src/components/ai-elements/", "ai/");
	console.log(`✗ ${short} (${f.compiled} compiled)`);
	for (const d of f.diagnostics) {
		console.log(`    line ${d.line}: ${d.short}`);
	}
}

if (failures.length === 0) {
	console.log("✓ All files compile cleanly.");
} else {
	process.exitCode = 1;
}
