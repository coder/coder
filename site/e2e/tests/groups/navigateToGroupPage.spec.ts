import { test, expect } from "@playwright/test";
import { createGroup, getCurrentOrgId, setupApiCalls } from "../../api";
import { requiresEnterpriseLicense } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("navigate to group page", async ({ page, baseURL }) => {
  requiresEnterpriseLicense();
  await setupApiCalls(page);
  const orgId = await getCurrentOrgId();
  const group = await createGroup(orgId);

  await page.goto(`${baseURL}/users`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Users - Coder");

  await page.getByRole("link", { name: "Groups" }).click();
  await expect(page).toHaveTitle("Groups - Coder");

  const groupRow = page.getByRole("row", { name: group.display_name });
  await groupRow.click();
  await expect(page).toHaveTitle(`${group.display_name} - Coder`);
});
