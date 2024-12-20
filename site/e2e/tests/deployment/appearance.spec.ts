import { chromium, expect, test } from "@playwright/test";
import { expectUrl } from "../../expectUrl";
import { login, randomName, requiresLicense } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("set application name", async ({ page }) => {
	requiresLicense();

	await page.goto("/deployment/appearance", { waitUntil: "domcontentloaded" });

	const applicationName = randomName();

	// Fill out the form
	const form = page.locator("form", { hasText: "Application name" });
	await form
		.getByLabel("Application name", { exact: true })
		.fill(applicationName);
	await form.getByRole("button", { name: "Submit" }).click();

	// Open a new session without cookies to see the login page
	const browser = await chromium.launch();
	const incognitoContext = await browser.newContext();
	await incognitoContext.clearCookies();
	const incognitoPage = await incognitoContext.newPage();
	await incognitoPage.goto("/", { waitUntil: "domcontentloaded" });

	// Verify the application name
	const name = incognitoPage.locator("h1", { hasText: applicationName });
	await expect(name).toBeVisible();

	// Shut down browser
	await incognitoPage.close();
	await browser.close();
});

test("set application logo", async ({ page }) => {
	requiresLicense();

	await page.goto("/deployment/appearance", { waitUntil: "domcontentloaded" });

	const imageLink = "/icon/azure.png";

	// Fill out the form
	const form = page.locator("form", { hasText: "Logo URL" });
	await form.getByLabel("Logo URL", { exact: true }).fill(imageLink);
	await form.getByRole("button", { name: "Submit" }).click();

	// Open a new session without cookies to see the login page
	const browser = await chromium.launch();
	const incognitoContext = await browser.newContext();
	await incognitoContext.clearCookies();
	const incognitoPage = await incognitoContext.newPage();
	await incognitoPage.goto("/", { waitUntil: "domcontentloaded" });

	// Verify banner
	const logo = incognitoPage.locator("img.application-logo");
	await expect(logo).toHaveAttribute("src", imageLink);

	// Shut down browser
	await incognitoPage.close();
	await browser.close();
});

test("set service banner", async ({ page }) => {
	requiresLicense();

	await page.goto("/deployment/appearance", { waitUntil: "domcontentloaded" });

	const message = "Mary has a little lamb.";

	// Fill out the form
	await page.getByRole("button", { name: "New" }).click();
	const form = page.getByRole("presentation");
	await form.getByLabel("Message", { exact: true }).fill(message);
	await form.getByRole("button", { name: "Update" }).click();

	// Verify service banner
	await page.goto("/workspaces", { waitUntil: "domcontentloaded" });
	await expectUrl(page).toHavePathName("/workspaces");

	const bar = page.locator("div.service-banner", { hasText: message });
	await expect(bar).toBeVisible();
});
