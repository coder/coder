import { chromium, expect, test } from "@playwright/test";
import { randomName, requiresEnterpriseLicense } from "../../helpers";

test("set application name", async ({ page }) => {
  requiresEnterpriseLicense();

  await page.goto("/deployment/appearance", { waitUntil: "domcontentloaded" });

  const applicationName = randomName();

  // Fill out the form
  const form = page.locator("form", { hasText: "Application name"})
  await form.getByLabel("Application name", { exact: true}).fill(applicationName);
  await form.getByRole("button", { name: "Submit"}).click();

  // Open a new session without cookies to see the login page
  const browser = await chromium.launch()
  const incognitoContext = await browser.newContext()
  await incognitoContext.clearCookies()
  const incognitoPage = await incognitoContext.newPage()
  await incognitoPage.goto("/", { waitUntil: "domcontentloaded" });

  const banner = incognitoPage.locator("h1", { hasText: applicationName})
  await expect(banner).toBeVisible()

  // Shut down browser
  await incognitoPage.close()
  await browser.close()
});
