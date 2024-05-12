import { test, expect, type Page } from "@playwright/test";
import { API } from "api/api";
import { setupApiCalls } from "../../api";
import { coderPort, workspaceProxyPort } from "../../constants";
import { randomName, requiresEnterpriseLicense } from "../../helpers";
import { startWorkspaceProxy, stopWorkspaceProxy } from "../../proxy";

test("default proxy is online", async ({ page }) => {
  requiresEnterpriseLicense();
  await setupApiCalls(page);

  await page.goto("/deployment/workspace-proxies", {
    waitUntil: "domcontentloaded",
  });

  // Verify if the default proxy is healthy
  const workspaceProxyPrimary = page.locator(
    `table.MuiTable-root tr[data-testid="primary"]`,
  );

  const workspaceProxyName = workspaceProxyPrimary.locator("td.name span");
  const workspaceProxyURL = workspaceProxyPrimary.locator("td.url");
  const workspaceProxyStatus = workspaceProxyPrimary.locator("td.status span");

  await expect(workspaceProxyName).toHaveText("Default");
  await expect(workspaceProxyURL).toHaveText("http://localhost:" + coderPort);
  await expect(workspaceProxyStatus).toHaveText("Healthy");
});

test("custom proxy is online", async ({ page }) => {
  requiresEnterpriseLicense();
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

  const workspaceProxy = page.locator(`table.MuiTable-root tr`, {
    hasText: proxyName,
  });

  const workspaceProxyName = workspaceProxy.locator("td.name span");
  const workspaceProxyURL = workspaceProxy.locator("td.url");
  const workspaceProxyStatus = workspaceProxy.locator("td.status span");

  await expect(workspaceProxyName).toHaveText(proxyName);
  await expect(workspaceProxyURL).toHaveText(
    `http://127.0.0.1:${workspaceProxyPort}`,
  );
  await expect(workspaceProxyStatus).toHaveText("Healthy");

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

    const workspaceProxy = page.locator(`table.MuiTable-root tr`, {
      hasText: proxyName,
    });
    const workspaceProxyStatus = workspaceProxy.locator("td.status span");

    try {
      await expect(workspaceProxyStatus).toHaveText("Healthy", {
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
