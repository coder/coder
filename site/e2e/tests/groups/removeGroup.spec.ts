import { test, expect } from "@playwright/test";
import { createGroup, getCurrentOrgId, setupApiCalls } from "../../api";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("remove group", async ({ page, baseURL }) => {
  await setupApiCalls(page);
  const orgId = await getCurrentOrgId();
  const group = await createGroup(orgId);

  await page.goto(`${baseURL}/groups`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Groups - Coder");

  const groupRow = page.getByRole("row", { name: group.display_name });
  await groupRow.click();

  await expect(page).toHaveTitle(`${group.display_name} - Coder`);
  await page.getByRole("button", { name: "Delete" }).click();

  const dialog = page.getByTestId("dialog");
  await dialog.getByLabel("Name of the group to delete").fill(group.name);
  await dialog.getByRole("button", { name: "Delete" }).click();
  await expect(page.getByText("Group deleted successfully.")).toBeVisible();

  await expect(page).toHaveTitle("Groups - Coder");
});
