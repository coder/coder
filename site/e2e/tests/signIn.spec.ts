/**
 * Thinking things through:
 * 1. Want to make sure that the user we're running the logout tests for is
 *    different from the user that is being used for the rest of the tests
 * 2. But we still need to use that initial user to make the request for making
 *    the new user
 * 3. But the specific Axios instance will still be configured for the previous
 *    user, so you need to change the token (and we have a method for that)
 *    1. Even if the backend isn't necessarily isolated, Playwright should
 *       basically have a different frontend per test, so changing the token for
 *       the sign-in test shouldn't have ripple effects for the rest of the
 *       tests
 *    2. My separate client refactor PR should theoretically help with this a
 *       bit, too, but I don't think it's needed
 *    3. Though actually, does it work like that? The Axios instance is still a
 *       global value. I doubt that Playwright is recreating the entire api.ts
 *       file environment on each test run
 *    4. At some point, this could be an architecture bottleneck, and we might
 *       have to figure out a way to create an Axios instance per test case.
 *    5. Maybe make a React provider that will instantiate a new client on
 *       mount,  and then expose it to the rest of the app via Context?
 *       - It doesn't sound terrible, but the tradeoff is that a bunch of files
 *         would need to be updated to support the explicit parameter
 * 4. Another the big problem is that I need to log out the initial user's
 *    account, but I don't know how to do that without sending an API request
 *    that will affect all the other tests
 */
import { test, expect } from "@playwright/test";
import { accessibleDropdownLabel } from "modules/dashboard/Navbar/UserDropdown/UserDropdown";
import { Language } from "modules/dashboard/Navbar/UserDropdown/UserDropdownContent";
import { createUser, getCurrentOrgId, setupApiCalls } from "../api";
import * as constants from "../constants";
import { assertNoUncaughtRenderError } from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

// One complication with this test is that you can't process the logout using
// the same user credentials as all the other test cases. If this runs first and
// logs the user out, every single other test will start failing, but Playwright
// also doesn't give you the ability to have certain tests run last. Have to
// make a separate user to guarantee more test isolation
const credentials = {
  username: "blah-blah-blah",
  password: constants.password,
} as const;

test.skip("Signing in and out", async ({ page, baseURL }) => {
  const handleSignIn = async () => {
    await page.goto(`${baseURL}/login`, { waitUntil: "domcontentloaded" });

    const emailField = page.getByRole("textbox", { name: /Email/ });
    const passwordField = page.getByRole("textbox", { name: /Password/ });
    const loginButton = page.getByRole("button", { name: /Sign in/i });

    await emailField.fill(`${credentials.username}@coder.com`);
    await passwordField.fill(credentials.password);
    await loginButton.click();

    await expect(page).toHaveURL(`${baseURL}/workspaces`);
  };

  const handleSignOut = async () => {
    const dropdownName = new RegExp(accessibleDropdownLabel);
    const dropdown = page.getByRole("button", { name: dropdownName });
    await dropdown.click();

    const signOutOption = page.getByRole("menuitem", {
      name: Language.signOutLabel,

      // Need to set to true because MUI automatically hides non-focused menu
      // items from screen readers (and by extension, Playwright). Trying to tab
      // through everything felt like it could get even more flaky
      includeHidden: true,
    });

    await signOutOption.click();
    await expect(page).toHaveTitle(/^Sign in to /);
    const atLoginPage = page.url().includes(`${baseURL}/login`);
    expect(atLoginPage).toBe(true);

    /**
     * 2024-05-06 - Adding this to assert that we can't have regressions around
     * the log out flow after it was fixed.
     * @see {@link https://github.com/coder/coder/issues/13130}
     */
    await assertNoUncaughtRenderError(page);
  };

  const setupNewUser = async () => {
    const orgId = await getCurrentOrgId();
    await createUser(orgId, credentials.username, credentials.password);
  };

  await setupApiCalls(page);
  await setupNewUser();

  await handleSignIn();
  await handleSignOut();
});
