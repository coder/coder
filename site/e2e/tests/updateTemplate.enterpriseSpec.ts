import { expect, test } from "@playwright/test";
import { skipEnterpriseTests } from "../constants";
import { createTemplate } from "../helpers";

test("require latest version", async ({ page }) => {
  test.skip(skipEnterpriseTests);
  const templateName = await createTemplate(page);

  await page.goto(`/templates/${templateName}/settings`, {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL(`/templates/${templateName}/settings`);
  let checkbox = await page.waitForSelector("#require_active_version");
  await checkbox.click();
  await page.getByTestId("form-submit").click();

  await page.goto(`/templates/${templateName}/settings`, {
    waitUntil: "domcontentloaded",
  });
  checkbox = await page.waitForSelector("#require_active_version");
  await checkbox.scrollIntoViewIfNeeded();
  expect(await checkbox.isChecked()).toBe(true);
});
