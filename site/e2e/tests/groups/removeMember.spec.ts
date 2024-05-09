import { test, expect } from "@playwright/test";
import { API } from "api/api";
import {
  createGroup,
  createUser,
  getCurrentOrgId,
  setupApiCalls,
} from "../../api";
import { requiresEnterpriseLicense } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("remove member", async ({ page, baseURL }) => {
  requiresEnterpriseLicense();
  await setupApiCalls(page);
  const orgId = await getCurrentOrgId();
  const [group, member] = await Promise.all([
    createGroup(orgId),
    createUser(orgId),
  ]);
  await API.addMember(group.id, member.id);

  await page.goto(`${baseURL}/groups/${group.id}`, {
    waitUntil: "domcontentloaded",
  });
  await expect(page).toHaveTitle(`${group.display_name} - Coder`);

  const userRow = page.getByRole("row", { name: member.username });
  await userRow.getByRole("button", { name: "More options" }).click();

  const menu = page.locator("#more-options");
  await menu.getByText("Remove").click({ timeout: 1_000 });

  await expect(page.getByText("Member removed successfully.")).toBeVisible();
});
