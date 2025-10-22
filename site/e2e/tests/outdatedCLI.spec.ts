import { randomUUID } from "node:crypto";
import { test } from "@playwright/test";
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

// we no longer support versions prior to Tailnet v2 API support: https://github.com/coder/coder/commit/059e533544a0268acbc8831006b2858ead2f0d8e
const clientVersion = "v2.8.0";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test(`ssh with client ${clientVersion}`, async ({ page }) => {
	const token = randomUUID();
	const template = await createTemplate(page, {
		apply: [
			{
				apply: {
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
	const binaryPath = await downloadCoderVersion(clientVersion);

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
