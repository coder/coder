import { expect, test } from "@playwright/test";
import { setupApiCalls } from "../api";
import { expectUrl } from "../expectUrl";
import { login, randomName, requiresLicense } from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test("create and delete organization", async ({ page }) => {
	requiresLicense();

	// Create an organization
	await page.goto("/organizations/new", {
		waitUntil: "domcontentloaded",
	});

	const name = randomName();
	await page.getByLabel("Slug").fill(name);
	await page.getByLabel("Display name").fill(`Org ${name}`);
	await page.getByLabel("Description").fill(`Org description ${name}`);
	await page.getByLabel("Icon", { exact: true }).fill("/emojis/1f957.png");
	await page.getByRole("button", { name: /save/i }).click();

	// Expect to be redirected to the new organization
	await expectUrl(page).toHavePathName(`/organizations/${name}`);
	await expect(page.getByText("Organization created.")).toBeVisible();

	await page.goto(`/organizations/${name}/settings`, {
		waitUntil: "domcontentloaded",
	});

	const newName = randomName();
	await page.getByLabel("Slug").fill(newName);
	await page.getByLabel("Description").fill(`Org description ${newName}`);
	await page.getByRole("button", { name: /save/i }).click();

	// Expect to be redirected when renaming the organization
	await expectUrl(page).toHavePathName(`/organizations/${newName}/settings`);
	await expect(page.getByText("Organization settings updated.")).toBeVisible();

	await page.goto(`/organizations/${newName}/settings`, {
		waitUntil: "domcontentloaded",
	});
	// Expect to be redirected when renaming the organization
	await expectUrl(page).toHavePathName(`/organizations/${newName}/settings`);

	await page.getByRole("button", { name: "Delete this organization" }).click();
	const dialog = page.getByTestId("dialog");
	await dialog.getByLabel("Name").fill(newName);
	await dialog.getByRole("button", { name: "Delete" }).click();
	await page.waitForTimeout(1000);
	await expect(page.getByText("Organization deleted")).toBeVisible();
});
