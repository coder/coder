import { test, expect } from "@playwright/test";
import { createUser, getCurrentOrgId, setupApiCalls } from "../../api";
import { requiresEnterpriseLicense } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

const DEFAULT_GROUP_NAME = "Everyone";

test(`Every user should be automatically added to the default '${DEFAULT_GROUP_NAME}' group upon creation`, async ({
  page,
  baseURL,
}) => {
  requiresEnterpriseLicense();
  await setupApiCalls(page);
  const orgId = await getCurrentOrgId();
  const numberOfMembers = 3;
  const users = await Promise.all(
    Array.from({ length: numberOfMembers }, () => createUser(orgId)),
  );

  await page.goto(`${baseURL}/groups`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Groups - Coder");

  const groupRow = page.getByRole("row", { name: DEFAULT_GROUP_NAME });
  await groupRow.click();
  await expect(page).toHaveTitle(`${DEFAULT_GROUP_NAME} - Coder`);

  for (const user of users) {
    await expect(page.getByRole("row", { name: user.username })).toBeVisible();
  }
});
