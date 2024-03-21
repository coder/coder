import { expect, test } from "@playwright/test";
import { createTemplate, updateTemplateSettings } from "../helpers";

test("template update with new name redirects on successful submit", async ({
  page,
}) => {
  const templateName = await createTemplate(page);

  await updateTemplateSettings(page, templateName, {
    name: "new-name",
  });
});

test.only("require latest version", async ({ page }) => {
  const templateName = await createTemplate(page);

  await page.goto(`/templates/${templateName}/settings`, {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL(`/templates/${templateName}/settings`);

  await (await page.waitForSelector("#require_active_version")).click();
  const ocheckbox = await page.waitForSelector("#require_active_version");
  expect(await ocheckbox.isChecked()).toBe(true);
  await page.getByTestId("form-submit").click();

  await page.goto(`/templates/${templateName}/settings`, {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveURL(`/templates/${templateName}/settings`);
  await page.reload();
  const checkbox = await page.waitForSelector("#require_active_version");
  await checkbox.scrollIntoViewIfNeeded();
  await new Promise((resolve) => setTimeout(resolve, 3000));
  expect(await checkbox.isChecked()).toBe(true);
});
