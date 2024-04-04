import { test, expect } from "@playwright/test";
import { randomName } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("create user with password", async ({ page, baseURL }) => {
  await page.goto(`${baseURL}/users`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Users - Coder");

  await page.getByRole("button", { name: "Create user" }).click();
  await expect(page).toHaveTitle("Create User - Coder");

  const name = randomName();
  const userValues = {
    username: name,
    email: `${name}@coder.com`,
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

  await expect(page).toHaveTitle("Users - Coder");
  await expect(page.locator("tr", { hasText: userValues.email })).toBeVisible();
});
