import { expect, test } from "@playwright/test";
import * as API from "api/api";
import { setupApiCalls } from "../../api";

test("enabled security settings", async ({ page }) => {
  await setupApiCalls(page);

  await page.goto("/deployment/security", { waitUntil: "domcontentloaded" });

  // Load deployment settings
  const config = await API.getDeploymentConfig();

  // Check flags

});
