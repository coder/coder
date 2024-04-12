import { expect, test } from "@playwright/test";
import { expectUrl } from "../expectUrl";
import {
  createGroup,
  createTemplate,
  requiresEnterpriseLicense,
  updateTemplateSettings,
} from "../helpers";

test("template update with new name redirects on successful submit", async ({
  page,
}) => {
  const templateName = await createTemplate(page);

  await updateTemplateSettings(page, templateName, {
    name: "new-name",
  });
});

test("add and remove a group", async ({ page }) => {
  requiresEnterpriseLicense();

  const templateName = await createTemplate(page);
  const groupName = await createGroup(page);

  await page.goto(`/templates/${templateName}/settings/permissions`, {
    waitUntil: "domcontentloaded",
  });
  await expectUrl(page).toHavePathName(
    `/templates/${templateName}/settings/permissions`,
  );

  // Type the first half of the group name
  await page
    .getByPlaceholder("Search for user or group", { exact: true })
    .fill(groupName.slice(0, 4));

  // Select the group from the list and add it
  await page.getByText(groupName).click();
  await page.getByText("Add member").click();
  const row = page.locator(".MuiTableRow-root", { hasText: groupName });
  await expect(row).toBeVisible();

  // Now remove the group
  await row.getByLabel("More options").click();
  await page.getByText("Remove").click();
  await expect(page.getByText("Group removed successfully!")).toBeVisible();
  await expect(row).not.toBeVisible();
});

test("require latest version", async ({ page }) => {
  requiresEnterpriseLicense();

  const templateName = await createTemplate(page);

  await page.goto(`/templates/${templateName}/settings`, {
    waitUntil: "domcontentloaded",
  });
  await expectUrl(page).toHavePathName(`/templates/${templateName}/settings`);
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
