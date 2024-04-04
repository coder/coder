import { test, expect } from "@playwright/test";
import {
  createGroup,
  createUser,
  getCurrentOrgId,
  setupApiCalls,
} from "../../api";
import { requiresEnterpriseLicense } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("add members", async ({ page, baseURL }) => {
  requiresEnterpriseLicense();
  await setupApiCalls(page);
  const orgId = await getCurrentOrgId();
  const group = await createGroup(orgId);
  const numberOfMembers = 3;
  const users = await Promise.all(
    Array.from({ length: numberOfMembers }, () => createUser(orgId)),
  );

  await page.goto(`${baseURL}/groups/${group.id}`, {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveTitle(`${group.display_name} - Coder`);

  for (const user of users) {
    await page.getByPlaceholder("User email or username").fill(user.username);
    await page.getByRole("option", { name: user.email }).click();
    await page.getByRole("button", { name: "Add user" }).click();
    await expect(page.getByRole("row", { name: user.username })).toBeVisible();
  }
});
