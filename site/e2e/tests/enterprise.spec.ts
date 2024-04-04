import { expect, test } from "@playwright/test";
import { requiresEnterpriseLicense } from "../helpers";

test("license was added successfully", async ({ page }) => {
  requiresEnterpriseLicense();

  await page.goto("/deployment/licenses", { waitUntil: "domcontentloaded" });
  const license = page.locator(".MuiPaper-root", { hasText: "#1" });
  await expect(license).toBeVisible();
});
