import { expect, test } from "@playwright/test";
import { API } from "api/api";
import { Language } from "pages/CreateUserPage/CreateUserForm";
import { setupApiCalls } from "./api";
import * as constants from "./constants";
import { expectUrl } from "./expectUrl";
import { storageState } from "./playwright.config";

test("setup deployment", async ({ page }) => {
	await page.goto("/", { waitUntil: "domcontentloaded" });
	await setupApiCalls(page);
	const exists = await API.hasFirstUser();
	// First user already exists, abort early. All tests execute this as a dependency,
	// if you run multiple tests in the UI, this will fail unless we check this.
	if (exists) {
		return;
	}

	// Setup first user
	await page.getByLabel(Language.usernameLabel).fill(constants.username);
	await page.getByLabel(Language.emailLabel).fill(constants.email);
	await page.getByLabel(Language.passwordLabel).fill(constants.password);
	await page.getByTestId("create").click();

	await expectUrl(page).toHavePathName("/workspaces");
	await page.context().storageState({ path: storageState });

	await page.getByTestId("button-select-template").isVisible();

	// Setup license
	if (constants.premiumTestsRequired || constants.license) {
		// Make sure that we have something that looks like a real license
		expect(constants.license).toBeTruthy();
		expect(constants.license.length).toBeGreaterThan(92); // the signature alone should be this long
		expect(constants.license.split(".").length).toBe(3); // otherwise it's invalid

		await page.goto("/deployment/licenses", { waitUntil: "domcontentloaded" });
		await expect(page).toHaveTitle("License Settings - Coder");

		await page.getByText("Add a license").click();
		await page.getByRole("textbox").fill(constants.license);
		await page.getByText("Upload License").click();

		await expect(
			page.getByText("You have successfully added a license"),
		).toBeVisible();
	}
});
