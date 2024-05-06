import { test, expect } from "@playwright/test";
import { accessibleDropdownLabel } from "modules/dashboard/Navbar/UserDropdown/UserDropdown";
import { Language } from "modules/dashboard/Navbar/UserDropdown/UserDropdownContent";
import { getApplicationName } from "utils/appearance";
import { setupApiCalls } from "../api";
import { assertNoUncaughtRuntimeError } from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

// Test assumes that global setup will automatically handle the sign in process
test("Signing out", async ({ page, baseURL }) => {
  await setupApiCalls(page);
  await page.goto("/");

  const dropdownName = new RegExp(accessibleDropdownLabel);
  const dropdown = page.getByRole("button", { name: dropdownName });
  await dropdown.click();

  const signOutOption = page.getByText(Language.signOutLabel);
  await signOutOption.click();

  const applicationName = getApplicationName();
  await expect(page).toHaveTitle(`Sign in to ${applicationName}`);
  await expect(page).toHaveURL(`${baseURL}/login`);

  /**
   * 2024-05-02 - Adding this to assert that we can't have regressions around
   * the log out flow after it was fixed.
   * @see {@link https://github.com/coder/coder/issues/13130}
   */
  await assertNoUncaughtRuntimeError(page);
});
