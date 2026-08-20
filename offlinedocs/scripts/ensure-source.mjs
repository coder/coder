// Guard against a fumadocs-mdx generation flake.
//
// `fumadocs-mdx` regenerates the `.source/` entry files (`server.ts`,
// `browser.ts`, ...) from `content/docs`. Intermittently, when it regenerates
// over an existing `.source/` (in CI the `postinstall` run generates a stub
// first, then `lint`/`build` regenerate after `sync`), it writes an empty
// `.source/server.ts`. `tsc` then fails with a confusing
// `TS2306: File '.../.source/server.ts' is not a module`, even though nothing
// in the tracked source changed. `.source/` is a gitignored generated artifact,
// so the failure is transient and clears on a re-generate.
//
// This runs right after `fumadocs-mdx` in the lint pipeline: it verifies the
// generated entry files are non-empty and re-generates a few times if not, so
// `tsc` never runs against a half-written `.source/`. In the normal case the
// files are already populated and this is just a couple of stat calls.
import { execSync } from "node:child_process";
import { statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

// `server.ts` is the file `tsc` imports through the `collections/server` alias
// and the only one the empty-file flake has been seen on; `browser.ts` is
// checked too so a partial generation cannot slip through.
const GENERATED = [".source/server.ts", ".source/browser.ts"];
const MAX_ATTEMPTS = 3;

function firstBad() {
	for (const rel of GENERATED) {
		let size = -1;
		try {
			size = statSync(resolve(root, rel)).size;
		} catch {
			size = -1;
		}
		if (size <= 0) {
			return { rel, missing: size < 0 };
		}
	}
	return null;
}

let bad = firstBad();
for (let attempt = 1; bad && attempt <= MAX_ATTEMPTS; attempt++) {
	console.warn(
		`[ensure-source] ${bad.rel} is ${bad.missing ? "missing" : "empty"}; ` +
			`re-running fumadocs-mdx (attempt ${attempt}/${MAX_ATTEMPTS})`,
	);
	execSync("fumadocs-mdx", { cwd: root, stdio: "inherit" });
	bad = firstBad();
}

if (bad) {
	console.error(
		`[ensure-source] ${bad.rel} is still ${bad.missing ? "missing" : "empty"} ` +
			`after ${MAX_ATTEMPTS} attempts; aborting before tsc.`,
	);
	process.exit(1);
}
