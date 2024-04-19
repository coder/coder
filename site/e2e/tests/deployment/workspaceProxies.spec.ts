import { test, expect } from "@playwright/test";
import { createWorkspaceProxy } from "api/api";
import { setupApiCalls } from "../../api";
import { coderPort } from "../../constants";
import { randomName, requiresEnterpriseLicense } from "../../helpers";

test("default proxy is online", async ({ page }) => {
  requiresEnterpriseLicense();

  await setupApiCalls(page);

  await page.goto("/deployment/workspace-proxies", {
    waitUntil: "domcontentloaded",
  });

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
  const proxyResponse = await createWorkspaceProxy({
    name: proxyName,
    display_name: "",
    icon: "/emojis/1f1e7-1f1f7.png",
  });
  expect(proxyResponse.proxy_token).not.toHaveLength(4);

  await page.goto("/deployment/workspace-proxies", {
    waitUntil: "domcontentloaded",
  });

  const workspaceProxyPrimary = page.locator(`table.MuiTable-root tr`, {
    hasText: proxyName,
  });

  const workspaceProxyName = workspaceProxyPrimary.locator("td.name span");
  //const workspaceProxyURL = workspaceProxyPrimary.locator("td.url");
  const workspaceProxyStatus = workspaceProxyPrimary.locator("td.status span");

  await expect(workspaceProxyName).toHaveText(proxyName);
  //await expect(workspaceProxyURL).toHaveText("http://localhost:" + coderPort);
  await expect(workspaceProxyStatus).toHaveText("Never seen");
});
