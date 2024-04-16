import { test } from "@playwright/test";
import * as API from "api/api";
import { setupApiCalls } from "../../api";

test("user authentication", async ({ page }) => {
  await setupApiCalls(page);

  await API.getDeploymentConfig();
  await page.goto("/deployment/userauth", { waitUntil: "domcontentloaded" });

  await page.pause();
});
