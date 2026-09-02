// pixel-storybook 0.3.0 reports failed builds to the Pixel platform but its
// CLI always exits 0, which keeps the CI job green. The runner decides the
// build verdict internally (failed snapshots, report errors, upload
// failures) and only reveals it in its final PATCH to the platform, so
// observe that request and mirror the verdict into the exit code. The
// package export map hides the internal modules, so they are imported by
// file path. Drop this wrapper once the CLI can exit nonzero on failure.
import { configure } from "../node_modules/@coder/pixel-storybook/build/config.js";
import { run } from "../node_modules/@coder/pixel-storybook/build/runner.js";

let buildStatus;
const realFetch = globalThis.fetch;
globalThis.fetch = (url, init) => {
	if (init?.method === "PATCH" && String(url).endsWith("/api/build")) {
		buildStatus = JSON.parse(String(init.body)).status;
	}
	return realFetch(url, init);
};

await configure();
await run();
if (buildStatus !== "complete") {
	console.error(`pixel: build finished with status ${buildStatus ?? "unknown"}`);
	process.exitCode = 1;
}
