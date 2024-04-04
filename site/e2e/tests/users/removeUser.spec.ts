import { test, expect } from "@playwright/test";
import * as API from "api/api";
import { randomName, setupApiCalls } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("remove user", async ({ page, baseURL }) => {
  await setupApiCalls(page);
  const currentUser = await API.getAuthenticatedUser();
  const name = randomName();
  const user = await API.createUser({
    email: `${name}@coder.com`,
    username: name,
    password: "s3cure&password!",
    login_type: "password",
    disable_login: false,
    organization_id: currentUser.organization_ids[0],
  });

  await page.goto(`${baseURL}/users`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Users - Coder");

  const userRow = page.locator("tr", { hasText: user.email });
  await userRow.getByRole("button", { name: "More options" }).click();
  await userRow.getByText("Delete", { exact: false }).click();

  const dialog = page.getByTestId("dialog");
  await dialog.getByLabel("Name of the user to delete").fill(user.username);
  await dialog.getByRole("button", { name: "Delete" }).click();

  await expect(page.getByText("Successfully deleted the user.")).toBeVisible();
});
