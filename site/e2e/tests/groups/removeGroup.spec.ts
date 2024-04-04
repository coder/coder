import { test, expect } from "@playwright/test";
import * as API from "api/api";
import { randomName, setupApiCalls } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("remove group", async ({ page, baseURL }) => {
  await setupApiCalls(page);
  const currentUser = await API.getAuthenticatedUser();
  const name = randomName();
  const orgId = currentUser.organization_ids[0];
  const group = await API.createGroup(orgId, {
    name,
    display_name: `Display Name of ${name}`,
    avatar_url: "/emojis/1f60d.png",
    quota_allowance: 0,
  });

  await page.goto(`${baseURL}/groups`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Groups - Coder");

  const groupRow = page.locator("tr", { hasText: group.display_name });
  await groupRow.click();

  await expect(page).toHaveTitle(`${group.display_name} - Coder`);
  await page.getByRole("button", { name: "Delete" }).click();

  const dialog = page.getByTestId("dialog");
  await dialog.getByLabel("Name of the group to delete").fill(group.name);
  await dialog.getByRole("button", { name: "Delete" }).click();
  await expect(page.getByText("Group deleted successfully.")).toBeVisible();

  await expect(page).toHaveTitle("Groups - Coder");
});
