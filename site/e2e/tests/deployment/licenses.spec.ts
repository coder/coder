import { expect, test } from "@playwright/test";
import { requiresLicense } from "../../helpers";

test("license was added successfully", async ({ page }) => {
	requiresLicense();

	await page.goto("/deployment/licenses", { waitUntil: "domcontentloaded" });
	const firstLicense = page.locator(".licenses > .license-card", {
		hasText: "#1",
	});
	await expect(firstLicense).toBeVisible();

	// Trial vs. Enterprise?
	const accountType = firstLicense.locator(".account-type");
	await expect(accountType).toHaveText("Premium");

	// License should not be expired yet
	const licenseExpires = firstLicense.locator(".license-expires");
	const licenseExpiresDate = new Date(await licenseExpires.innerText());
	const now = new Date();
	expect(licenseExpiresDate.getTime()).toBeGreaterThan(now.getTime());

	// "Remove" button should be visible
	const removeButton = firstLicense.locator(".remove-button");
	await expect(removeButton).toBeVisible();
});
