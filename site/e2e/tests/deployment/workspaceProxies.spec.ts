import { type Page, expect, test } from "@playwright/test";
import { API } from "api/api";
import { setupApiCalls } from "../../api";
import { coderPort, workspaceProxyPort } from "../../constants";
import { randomName, requiresLicense } from "../../helpers";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";
import { startWorkspaceProxy, stopWorkspaceProxy } from "../../proxy";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("default proxy is online", async ({ page }) => {
	requiresLicense();
	await setupApiCalls(page);

	await page.goto("/deployment/workspace-proxies", {
		waitUntil: "domcontentloaded",
	});

	// Verify if the default proxy is healthy
	const workspaceProxyPrimary = page.locator(
		`table.MuiTable-root tr[data-testid="primary"]`,
	);

	const summary = workspaceProxyPrimary.locator(".summary");
	const status = workspaceProxyPrimary.locator(".status");

	await expect(summary).toContainText("Default");
	await expect(summary).toContainText(`http://localhost:${coderPort}`);
	await expect(status).toContainText("Healthy");
});

test("custom proxy is online", async ({ page }) => {
	requiresLicense();
	await setupApiCalls(page);

	const proxyName = randomName();

	// Register workspace proxy
	const proxyResponse = await API.createWorkspaceProxy({
		name: proxyName,
		display_name: "",
		icon: "/emojis/1f1e7-1f1f7.png",
	});
	expect(proxyResponse.proxy_token).toBeDefined();

	// Start "wsproxy server"
	const proxyServer = await startWorkspaceProxy(proxyResponse.proxy_token);
	await waitUntilWorkspaceProxyIsHealthy(page, proxyName);

	// Verify if custom proxy is healthy
	await page.goto("/deployment/workspace-proxies", {
		waitUntil: "domcontentloaded",
	});

	const proxyRow = page.locator("table.MuiTable-root tr", {
		hasText: proxyName,
	});

	const summary = proxyRow.locator(".summary");
	const status = proxyRow.locator(".status");

	await expect(summary).toContainText(proxyName);
	await expect(summary).toContainText(`http://127.0.0.1:${workspaceProxyPort}`);
	await expect(status).toContainText("Healthy");

	// Tear down the proxy
	await stopWorkspaceProxy(proxyServer);
});

const waitUntilWorkspaceProxyIsHealthy = async (
	page: Page,
	proxyName: string,
) => {
	await page.goto("/deployment/workspace-proxies", {
		waitUntil: "domcontentloaded",
	});

	const maxRetries = 30;
	const retryIntervalMs = 1000;
	let retries = 0;
	while (retries < maxRetries) {
		await page.reload();

		const proxyRow = page.locator("table.MuiTable-root tr", {
			hasText: proxyName,
		});
		const status = proxyRow.locator(".status");

		try {
			await expect(status).toContainText("Healthy", {
				timeout: 1_000,
			});
			return; // healthy!
		} catch {
			retries++;
			await new Promise((resolve) => setTimeout(resolve, retryIntervalMs));
		}
	}
	throw new Error(
		`Workspace proxy "${proxyName}" is unhealthy after  ${
			maxRetries * retryIntervalMs
		}ms`,
	);
};
