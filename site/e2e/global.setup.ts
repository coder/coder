import { test, expect } from "@playwright/test";
import { Language } from "pages/CreateUserPage/CreateUserForm";
import * as constants from "./constants";
import { STORAGE_STATE } from "./playwright.config";

test("setup first user", async ({ page }) => {
  await page.goto("/", { waitUntil: "domcontentloaded" });

  await page.getByLabel(Language.usernameLabel).fill(constants.username);
  await page.getByLabel(Language.emailLabel).fill(constants.email);
  await page.getByLabel(Language.passwordLabel).fill(constants.password);
  await page.getByTestId("create").click();

  await expect(page).toHaveURL(/\/workspaces.*/);
  await page.context().storageState({ path: STORAGE_STATE });

  await page.getByTestId("button-select-template").isVisible();
});
