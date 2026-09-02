// pixel-storybook 0.3.0 reports failed builds to the Pixel platform but its
// CLI always exits 0, which keeps the CI job green. Drive its internal
// modules directly and fail on any failed snapshot, mirroring the runner's
// own per-shot outcome rule. The package export map hides these modules, so
// they are imported by file path. Drop this wrapper once the CLI can exit
// nonzero on failure itself.
import { outcomeFromRenderStatus } from "../node_modules/@coder/pixel-storybook/build/api.js";
import { configure } from "../node_modules/@coder/pixel-storybook/build/config.js";
import { run } from "../node_modules/@coder/pixel-storybook/build/runner.js";

await configure();
const shots = await run();
const failed = shots.filter(
	(shot) =>
		outcomeFromRenderStatus(shot.render?.status ?? "timeout") === "fail",
);
if (failed.length > 0) {
	console.error(`pixel: ${failed.length} snapshot(s) failed`);
	process.exitCode = 1;
}
