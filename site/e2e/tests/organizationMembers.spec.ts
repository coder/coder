import { expect, test } from "@playwright/test";
import { setupApiCalls } from "../api";
import {
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
	const { displayName } = await createOrganization(page);

	// Navigate to members page
	await page.getByRole("link", { name: "Members" }).click();
	await expect(page).toHaveTitle(`Members - ${displayName} - Coder`);

	// Add a user to the org
	const personToAdd = await createUser(page);
	await page.getByPlaceholder("User email or username").fill(personToAdd.email);
	await page.getByRole("option", { name: personToAdd.email }).click();
	await page.getByRole("button", { name: "Add user" }).click();
	const addedRow = page.locator("tr", { hasText: personToAdd.email });
	await expect(addedRow).toBeVisible();

	// Give them a role
	await addedRow.getByLabel("Edit user roles").click();
	await page.getByText("Organization User Admin").click();
	await page.getByText("Organization Template Admin").click();
	await page.mouse.click(10, 10); // close the popover by clicking outside of it
	await expect(addedRow.getByText("Organization User Admin")).toBeVisible();
	await expect(addedRow.getByText("+1 more")).toBeVisible();

	// Remove them from the org
	await addedRow.getByLabel("More options").click();
	await page.getByText("Remove").click(); // Click the "Remove" option
	await page.getByRole("button", { name: "Remove" }).click(); // Click "Remove" in the confirmation dialog
	await expect(addedRow).not.toBeVisible();
});
