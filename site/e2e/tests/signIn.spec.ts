import { test, expect } from "@playwright/test";
import { accessibleDropdownLabel } from "modules/dashboard/Navbar/UserDropdown/UserDropdown";
import { getApplicationName } from "utils/appearance";
import { pageTitle } from "utils/page";
import { setupApiCalls } from "../api";
import { assertNoUncaughtRuntimeError } from "../helpers";
import { beforeCoderTest } from "../hooks";

const DUMMY_USERNAME = "admin";
const DUMMY_PASSWORD = "SomeSecurePassword!";
const applicationName = getApplicationName();

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("Sign in then sign out", async ({ page, baseURL }) => {
  const handleSignIn = async () => {
    // Should automatically be redirected to login page
    await page.goto(String(baseURL), { waitUntil: "domcontentloaded" });
    await expect(page).toHaveTitle(`Sign in to ${applicationName}`);

    // Assert that all elements for sign in flow are ready to go immediately
    const emailField = page.getByRole("textbox", { name: "Email" });
    const passwordField = page.getByRole("textbox", { name: "Password" });
    const signInButton = page.getByRole("button", { name: /Sign in/ });

    await emailField.click();
    await page.keyboard.type(DUMMY_USERNAME);
    await passwordField.click();
    await page.keyboard.type(DUMMY_PASSWORD);
    await signInButton.click();

    await expect(page).toHaveTitle(pageTitle("Workspaces"));
    await assertNoUncaughtRuntimeError(page);
  };

  const handleSignOut = async () => {
    const userDropdownButton = page.getByRole("button", {
      name: accessibleDropdownLabel,
    });

    await userDropdownButton.click();

    const signOutButton = page.getByRole("button", { name: /Sign Out/ });
    await signOutButton.click();

    await expect(page).toHaveTitle(`Sign in to ${applicationName}`);
    await assertNoUncaughtRuntimeError(page);
  };

  await setupApiCalls(page);
  await handleSignIn();
  await handleSignOut();
});
