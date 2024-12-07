import { expect, test } from "@playwright/test";
import { setupApiCalls } from "../api";
import { expectUrl } from "../expectUrl";
import { requiresLicense } from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => {
	await beforeCoderTest(page);
	await setupApiCalls(page);
});

test("create and delete organization", async ({ page }) => {
	requiresLicense();

	// Create an organization
	await page.goto("/organizations/new", {
		waitUntil: "domcontentloaded",
	});

	await page.getByLabel("Slug").fill("floop");
	await page.getByLabel("Display name").fill("Floop");
	await page.getByLabel("Description").fill("Org description floop");
	await page.getByLabel("Icon", { exact: true }).fill("/emojis/1f957.png");
	await page.getByRole("button", { name: "Submit" }).click();

	// Expect to be redirected to the new organization
	await expectUrl(page).toHavePathName("/organizations/floop");
	await expect(page.getByText("Organization created.")).toBeVisible();

	await page.getByLabel("Slug").fill("wibble");
	await page.getByLabel("Description").fill("Org description wibble");
	await page.getByRole("button", { name: "Submit" }).click();

	// Expect to be redirected when renaming the organization
	await expectUrl(page).toHavePathName("/organizations/wibble");
	await expect(page.getByText("Organization settings updated.")).toBeVisible();

	await page.getByRole("button", { name: "Delete this organization" }).click();
	const dialog = page.getByTestId("dialog");
	await dialog.getByLabel("Name").fill("wibble");
	await dialog.getByRole("button", { name: "Delete" }).click();
	await expect(page.getByText("Organization deleted.")).toBeVisible();
});
