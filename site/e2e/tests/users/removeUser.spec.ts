import { test, expect } from "@playwright/test";
import { createUser, getCurrentOrgId, setupApiCalls } from "../../api";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("remove user", async ({ page, baseURL }) => {
  await setupApiCalls(page);
  const orgId = await getCurrentOrgId();
  const user = await createUser(orgId);

  await page.goto(`${baseURL}/users`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Users - Coder");

  const userRow = page.getByRole("row", { name: user.email });
  await userRow.getByRole("button", { name: "More options" }).click();
  const menu = page.locator("#more-options");
  await menu.getByText("Delete").click();

  const dialog = page.getByTestId("dialog");
  await dialog.getByLabel("Name of the user to delete").fill(user.username);
  await dialog.getByRole("button", { name: "Delete" }).click();

  await expect(page.getByText("Successfully deleted the user.")).toBeVisible();
});
