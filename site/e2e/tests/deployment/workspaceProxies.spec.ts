import { test, expect } from "@playwright/test";
import { setupApiCalls } from "../../api";
import { coderPort } from "../../constants";
import { requiresEnterpriseLicense } from "../../helpers";

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
