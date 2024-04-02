import { test, expect } from "@playwright/test";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("manage users", async ({ page, baseURL }) => {
  await page.goto(`${baseURL}/users`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Users - Coder");

  await page.getByRole("button", { name: "Create user" }).click();
  await expect(page).toHaveTitle("Create User - Coder");

  const userValues = {
    username: "testuser",
    email: "testuser@coder.com",
    loginType: "password",
    password: "s3cure&password!",
  };
  await page.getByLabel("Username").fill(userValues.username);
  await page.getByLabel("Email").fill(userValues.email);
  await page.getByLabel("Login Type").click();
  await page.getByRole("option", { name: "Password", exact: false }).click();
  // Using input[name=password] due to the select element utilizing 'password'
  // as the label for the currently active option.
  const passwordField = page.locator("input[name=password]");
  await passwordField.fill(userValues.password);
  await page.getByRole("button", { name: "Create user" }).click();
  await expect(page.getByText("Successfully created user.")).toBeVisible();

  const userRow = page.locator("tr", { hasText: userValues.email });
  await userRow.getByRole("button", { name: "More options" }).click();
  await page.getByText("Delete", { exact: false }).click();
  await page.getByLabel("Name of the user to delete").fill(userValues.username);
  await page.getByRole("button", { name: "Delete" }).click();
  await expect(page.getByText("Successfully deleted the user.")).toBeVisible();
});
