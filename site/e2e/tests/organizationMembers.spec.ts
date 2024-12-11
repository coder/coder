import { expect, test } from "@playwright/test";
import { setupApiCalls } from "../api";
import { expectUrl } from "../expectUrl";
import { createUser, randomName, requiresLicense } from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => {
	await beforeCoderTest(page);
	await setupApiCalls(page);
});

test("add and remove organization member", async ({ page }) => {
	requiresLicense();

	// Create a new organization to test
	await page.goto("/organizations/new", { waitUntil: "domcontentloaded" });
	const name = randomName();
	await page.getByLabel("Slug").fill(name);
	await page.getByLabel("Display name").fill(`Org ${name}`);
	await page.getByLabel("Description").fill(`Org description ${name}`);
	await page.getByLabel("Icon", { exact: true }).fill("/emojis/1f957.png");
	await page.getByRole("button", { name: "Submit" }).click();

	// Navigate to members page
	await expectUrl(page).toHavePathName(`/organizations/${name}`);
	await expect(page.getByText("Organization created.")).toBeVisible();
	await page.getByText("Members").click();

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
