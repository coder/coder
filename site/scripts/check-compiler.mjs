import { readFileSync } from "node:fs";
import { execSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const siteDir = resolve(__dirname, "..");

const babel = await import(
	resolve(siteDir, "node_modules/.pnpm/@babel+core@7.28.5/node_modules/@babel/core/lib/index.js")
);
const syntaxTSPlugin = resolve(
	siteDir,
	"node_modules/.pnpm/@babel+plugin-syntax-typescript@7.24.7_@babel+core@7.28.5/node_modules/@babel/plugin-syntax-typescript/lib/index.js",
);

const files = execSync(
	"find src/pages/AgentsPage src/components/ai-elements -type f \\( -name '*.tsx' -o -name '*.ts' \\) ! -name '*.test.*' ! -name '*.stories.*' ! -name '*.jest.*'",
	{ encoding: "utf-8", cwd: siteDir },
).trim().split("\n").filter(Boolean);

let totalCompiled = 0;
const failures = [];

for (const file of files) {
	const code = readFileSync(resolve(siteDir, file), "utf-8");
	const isTSX = file.endsWith(".tsx");
	const diagnostics = [];

	try {
		const result = babel.transformSync(code, {
			plugins: [
				[syntaxTSPlugin, { isTSX }],
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
