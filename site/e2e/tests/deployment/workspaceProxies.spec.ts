import { test } from "@playwright/test";
import {
  setupApiCalls,
} from "../../api";

test("workspace proxies configuration", async ({ page }) => {
  await setupApiCalls(page);

  await page.goto("/deployment/workspace-proxies", {
    waitUntil: "domcontentloaded",
  });

  await page.pause();
});
