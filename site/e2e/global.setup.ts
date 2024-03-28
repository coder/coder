import { test, expect } from "@playwright/test";
import { Language } from "pages/CreateUserPage/CreateUserForm";
import * as constants from "./constants";
import { storageState } from "./playwright.config";

test("setup first user", async ({ page }) => {
  await page.goto("/", { waitUntil: "domcontentloaded" });

  // Setup first user
  await page.getByLabel(Language.usernameLabel).fill(constants.username);
  await page.getByLabel(Language.emailLabel).fill(constants.email);
  await page.getByLabel(Language.passwordLabel).fill(constants.password);
  await page.getByTestId("create").click();

  await expect(page).toHaveURL(/\/workspaces.*/);
  await page.context().storageState({ path: storageState });

  await page.getByTestId("button-select-template").isVisible();

  // Setup license
  if (constants.enterpriseLicense) {
    await page.goto("/deployment/licenses", { waitUntil: "domcontentloaded" });

    await page.getByText("Add a license").click();
    await page.getByRole("textbox").fill(constants.enterpriseLicense);
    await page.getByText("Upload License").click();

    await page.getByText("You have successfully added a license").isVisible();
  }
});
