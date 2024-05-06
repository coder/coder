import { test, expect } from "@playwright/test";
import { accessibleDropdownLabel } from "modules/dashboard/Navbar/UserDropdown/UserDropdown";
import { Language } from "modules/dashboard/Navbar/UserDropdown/UserDropdownContent";
import { setupApiCalls } from "../api";
import * as constants from "../constants";
import { assertNoUncaughtRuntimeError } from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("Signing out", async ({ page, baseURL }) => {
  const handleSignIn = async () => {
    await page.goto(`${baseURL}/login`, { waitUntil: "domcontentloaded" });

    if (!page.url().endsWith("/login")) {
      return;
    }

    const emailField = page.getByRole("textbox", { name: /Email/ });
    const passwordField = page.getByRole("textbox", { name: /Password/ });
    const loginButton = page.getByRole("button", { name: /Sign in/i });

    await emailField.fill(constants.email);
    await passwordField.fill(constants.password);
    await loginButton.click();

    await expect(page).toHaveURL(`${baseURL}/workspaces`);
  };

  const handleSignOut = async () => {
    const dropdownName = new RegExp(accessibleDropdownLabel);
    const dropdown = page.getByRole("button", { name: dropdownName });
    await dropdown.click({ timeout: 3_000 });

    const signOutOption = page.getByText(Language.signOutLabel);
    await signOutOption.click({ timeout: 3_000 });

    await expect(page).toHaveTitle(/^Sign in to /);
    const atLoginPage = page.url().includes(`${baseURL}/login`);
    expect(atLoginPage).toBe(true);

    /**
     * 2024-05-02 - Adding this to assert that we can't have regressions around
     * the log out flow after it was fixed.
     * @see {@link https://github.com/coder/coder/issues/13130}
     */
    await assertNoUncaughtRuntimeError(page);
  };

  await setupApiCalls(page);
  await handleSignIn();
  await handleSignOut();
});
