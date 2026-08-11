import { randomUUID } from "node:crypto";
import { test } from "@playwright/test";
import { oldestSupportedAgentVersion } from "../constants";
import {
	createTemplate,
	createWorkspace,
	downloadCoderVersion,
	login,
	sshIntoWorkspace,
	startAgentWithCommand,
	stopAgent,
	stopWorkspace,
} from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test.skip(`ssh with agent ${oldestSupportedAgentVersion}`, async ({ page }) => {
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
	const binaryPath = await downloadCoderVersion(oldestSupportedAgentVersion);
	const agent = await startAgentWithCommand(page, token, binaryPath);

	const client = await sshIntoWorkspace(page, workspaceName);
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
