import { expect, test } from "@playwright/test";
import { API } from "api/api";
import { Language } from "pages/CreateUserPage/Language";
import { coderPort, license, premiumTestsRequired, users } from "../constants";
import { expectUrl } from "../expectUrl";
import { createUser } from "../helpers";

test("setup deployment", async ({ page }) => {
	await page.goto("/", { waitUntil: "domcontentloaded" });
	API.setHost(`http://127.0.0.1:${coderPort}`);
	const exists = await API.hasFirstUser();
	// First user already exists, abort early. All tests execute this as a dependency,
	// if you run multiple tests in the UI, this will fail unless we check this.
	if (exists) {
		return;
	}

	// Setup first user
	await page.getByLabel(Language.usernameLabel).fill(users.admin.username);
	await page.getByLabel(Language.emailLabel).fill(users.admin.email);
	await page.getByLabel(Language.passwordLabel).fill(users.admin.password);
	await page.getByTestId("create").click();

	await expectUrl(page).toHavePathName("/workspaces");
	await page.getByTestId("button-select-template").isVisible();

	for (const user of Object.values(users)) {
		// Already created as first user
		if (user.username === "admin") {
			continue;
		}

		await createUser(page, user);
	}

	// Setup license
	if (premiumTestsRequired || license) {
		// Make sure that we have something that looks like a real license
		expect(license).toBeTruthy();
		expect(license.length).toBeGreaterThan(92); // the signature alone should be this long
		expect(license.split(".").length).toBe(3); // otherwise it's invalid

		await page.goto("/deployment/licenses", { waitUntil: "domcontentloaded" });
		await expect(page).toHaveTitle("License Settings - Coder");

		await page.getByText("Add a license").click();
		await page.getByRole("textbox").fill(license);
		await page.getByText("Upload License").click();

		await expect(
			page.getByText("You have successfully added a license"),
		).toBeVisible();
	}
});
