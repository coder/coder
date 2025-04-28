import { expect, test } from "@playwright/test";
import { setupApiCalls } from "../api";
import {
	addUserToOrganization,
	createOrganization,
	createUser,
	login,
	requiresLicense,
} from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test("add and remove organization member", async ({ page }) => {
	requiresLicense();

	// Create a new organization
	const { name: orgName, displayName } = await createOrganization(page);

	// Navigate to members page
	await page.getByRole("link", { name: "Members" }).click();
	await expect(page).toHaveTitle(`Members - ${displayName} - Coder`);

	// Add a user to the org
	const personToAdd = await createUser(page);
	// This must be done as an admin, because you can't assign a role that has more
	// permissions than you, even if you have the ability to assign roles.
	await addUserToOrganization(page, orgName, personToAdd.email, [
		"Organization User Admin",
		"Organization Template Admin",
	]);

	const addedRow = page.locator("tr", { hasText: personToAdd.email });
	await expect(addedRow.getByText("Organization User Admin")).toBeVisible();
	await expect(addedRow.getByText("+1 more")).toBeVisible();

	// Remove them from the org
	await addedRow.getByLabel("More options").click();
	await page.getByText("Remove").click(); // Click the "Remove" option
	await page.getByRole("button", { name: "Remove" }).click(); // Click "Remove" in the confirmation dialog
	await expect(addedRow).not.toBeVisible();
});
