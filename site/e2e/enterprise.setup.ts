import { test } from "@playwright/test";

const license = process.env.CODER_E2E_ENTERPRISE_LICENSE!;

test("setup license", async ({ page }) => {
  await page.goto("/deployment/licenses", { waitUntil: "domcontentloaded" });

  await page.getByText("Add a license").click();
  await page.getByRole("textbox").fill(license);
  await page.getByText("Upload License").click();

  await page.getByText("You have successfully added a license").isVisible();
});
