import { test } from "@playwright/test";
import { enterpriseLicense, skipEnterpriseTests } from "./constants";

test("setup license", async ({ page }) => {
  test.skip(skipEnterpriseTests);

  await page.goto("/deployment/licenses", { waitUntil: "domcontentloaded" });

  await page.getByText("Add a license").click();
  await page.getByRole("textbox").fill(enterpriseLicense);
  await page.getByText("Upload License").click();

  await page.getByText("You have successfully added a license").isVisible();
});
