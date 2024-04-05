import { expect, test } from "@playwright/test";
import { Language } from "pages/CreateUserPage/CreateUserForm";
import * as constants from "./constants";
import { storageState } from "./playwright.config";

test("setup deployment", async ({ page }) => {
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
  if (constants.requireEnterpriseTests || constants.enterpriseLicense) {
    // Make sure that we have something that looks like a real license
    expect(constants.enterpriseLicense).toBeTruthy();
    expect(constants.enterpriseLicense.length).toBeGreaterThan(92); // the signature alone should be this long
    expect(constants.enterpriseLicense.split(".").length).toBe(3); // otherwise it's invalid

    await page.goto("/deployment/licenses", { waitUntil: "domcontentloaded" });

    await page.getByText("Add a license").click();
    await page.getByRole("textbox").fill(constants.enterpriseLicense);
    await page.getByText("Upload License").click();

    await page.getByText("You have successfully added a license").isVisible();
  }
});
