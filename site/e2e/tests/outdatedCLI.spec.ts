import { randomUUID } from "node:crypto";
import { test } from "@playwright/test";
import { oldestSupportedCLIVersion } from "../constants";
import {
	createTemplate,
	createWorkspace,
	downloadCoderVersion,
	login,
	sshIntoWorkspace,
	startAgent,
	stopAgent,
	stopWorkspace,
} from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test(`ssh with client ${oldestSupportedCLIVersion}`, async ({ page }) => {
	// setup/downloadCoderVersions.spec.ts normally has the binary cached by now,
	// leaving this a local-only test. The extra headroom covers the case where
	// that prefetch failed and downloadCoderVersion has to fetch it inline.
	test.setTimeout(60_000);

	const token = randomUUID();
	const template = await createTemplate(page, {
		graph: [
			{
				graph: {
					resources: [
						{
							agents: [
								{
									token,
									order: 0,
								},
							],
						},
					],
				},
			},
		],
	});
	const workspaceName = await createWorkspace(page, template);
	const agent = await startAgent(page, token);
	const binaryPath = await downloadCoderVersion(oldestSupportedCLIVersion);

	const client = await sshIntoWorkspace(page, workspaceName, binaryPath);
	await new Promise<void>((resolve, reject) => {
		// We just exec a command to be certain the agent is running!
		client.exec("exit 0", (err, stream) => {
			if (err) {
				return reject(err);
			}
			stream.on("exit", (code) => {
				if (code !== 0) {
					return reject(new Error(`Command exited with code ${code}`));
				}
				client.end();
				resolve();
			});
		});
	});

	await stopWorkspace(page, workspaceName);
	await stopAgent(agent);
});
