import { test, expect } from "@playwright/test";
import * as constants from "./constants";
import { STORAGE_STATE } from "./playwright.config";
import { Language as CreateLanguage } from "../src/pages/CreateUserPage/CreateUserForm";
import { Language as LoginLanguage } from "../src/pages/LoginPage/SignInForm";

test("setup or sign in", async ({ page }) => {
  await page.goto("/", { waitUntil: "domcontentloaded" });

  await page.getByLabel(CreateLanguage.emailLabel).fill(constants.email);
  await page.getByLabel(CreateLanguage.passwordLabel).fill(constants.password);

  const signInButton = page.getByRole("button", {
    name: LoginLanguage.passwordSignIn,
  });
  if (await signInButton.isVisible()) {
    await signInButton.click();
  } else {
    await page
      .getByLabel(CreateLanguage.usernameLabel)
      .fill(constants.username);
    await page.getByTestId("trial").click();
    await page.getByTestId("create").click();
  }

  await expect(page).toHaveURL(/\/workspaces.*/);
  await page.context().storageState({ path: STORAGE_STATE });
});
