import { randomUUID } from "node:crypto";
import * as http from "node:http";
import { expect, test } from "@playwright/test";
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
	const appName = "test-app";

	// Start an HTTP server to act as the workspace app backend.
	const server = http.createServer((_req, res) => {
		res.writeHead(200, { "Content-Type": "text/plain" });
		res.end(appContent);
	});

	// Wait for the server to be fully listening before proceeding.
	// Using a callback avoids the race where address() is called
	// before the socket is bound.
	const port = await new Promise<number>((resolve, reject) => {
		server.on("error", reject);
		server.listen(0, () => {
			const addr = server.address();
			if (typeof addr !== "object" || !addr) {
				reject(new Error("Expected address to be an AddressInfo"));
				return;
			}
			resolve(addr.port);
		});
	});

	try {
		const template = await createTemplate(page, {
			graph: [
				{
					graph: {
						resources: [
							{
								agents: [
									{
										token,
										apps: [
											{
												id: randomUUID(),
												url: `http://localhost:${port}`,
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

		// Register the popup listener before clicking so we never miss
		// the event.
		const appPagePromise = context.waitForEvent("page");
		await page.getByRole("link", { name: appName }).click();
		const appPage = await appPagePromise;

		// SLIM_WINDOW opens about:blank first, then sets location.href
		// to the proxied app URL. A retrying assertion tolerates the
		// intermediate blank page and any app-proxy startup delay.
		await expect(appPage.getByText(appContent)).toBeVisible({
			timeout: 30_000,
		});

		await stopWorkspace(page, workspaceName);
		await stopAgent(agent);
	} finally {
		server.close();
	}
});
