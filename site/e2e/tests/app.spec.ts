import { randomUUID } from "node:crypto";
import * as http from "node:http";
import { test } from "@playwright/test";
import {
	createTemplate,
	createWorkspace,
	login,
	startAgent,
	stopAgent,
	stopWorkspace,
} from "../helpers";
import { beforeCoderTest } from "../hooks";
import { AppOpenIn } from "../provisionerGenerated";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("app", async ({ context, page }) => {
	const appContent = "Hello World";
	const token = randomUUID();
	const srv = http
		.createServer((req, res) => {
			res.writeHead(200, { "Content-Type": "text/plain" });
			res.end(appContent);
		})
		.listen(0);
	const addr = srv.address();
	if (typeof addr !== "object" || !addr) {
		throw new Error("Expected addr to be an object");
	}
	const appName = "test-app";
	const template = await createTemplate(page, {
		apply: [
			{
				apply: {
					resources: [
						{
							agents: [
								{
									token,
									apps: [
										{
											url: `http://localhost:${addr.port}`,
											displayName: appName,
											order: 0,
											openIn: AppOpenIn.SLIM_WINDOW,
										},
									],
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

	// Wait for the web terminal to open in a new tab
	const pagePromise = context.waitForEvent("page");
	await page.getByText(appName).click({ timeout: 10_000 });
	const app = await pagePromise;
	await app.waitForLoadState("domcontentloaded");
	await app.getByText(appContent).isVisible();

	await stopWorkspace(page, workspaceName);
	await stopAgent(agent);
});
