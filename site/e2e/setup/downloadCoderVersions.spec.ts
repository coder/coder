import { test } from "@playwright/test";
import { oldestSupportedCLIVersion } from "../constants";
import { downloadCoderVersion } from "../helpers";

// Fetching a release binary is network-bound and effectively unbounded: the
// install script downloads an 84 MiB asset and retries with backoff. Doing it
// here rather than inside the compatibility test keeps that time off the test's
// timeout, so a slow GitHub response can no longer surface as an SSH failure.
test("download outdated Coder CLI", async () => {
	// Generous because this covers a cold download plus install.sh's retries.
	test.setTimeout(300_000);

	// A failure here is deliberately not fatal. Tests in the `tests` project
	// depend on this project, and a failing dependency stops all of them from
	// running, so a GitHub outage would block the entire suite rather than the
	// one test that needs this binary. outdatedCLI.spec.ts calls
	// downloadCoderVersion itself, so it retries inline and fails alone.
	try {
		await downloadCoderVersion(oldestSupportedCLIVersion);
	} catch (error) {
		console.error(
			`Failed to prefetch the Coder ${oldestSupportedCLIVersion} CLI. ` +
				"outdatedCLI.spec.ts will download it inline and may time out.",
			error,
		);
	}
});
