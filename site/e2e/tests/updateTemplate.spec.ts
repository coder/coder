import { expect, test } from "@playwright/test";
import { defaultOrganizationName, users } from "../constants";
import { expectUrl } from "../expectUrl";
import {
	createGroup,
	createTemplate,
	login,
	requiresLicense,
	updateTemplateSettings,
} from "../helpers";
import { beforeCoderTest } from "../hooks";

test.describe.configure({ mode: "parallel" });

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page, users.templateAdmin);
});

test("template update with new name redirects on successful submit", async ({
	page,
}) => {
	const templateName = await createTemplate(page);
	await updateTemplateSettings(page, templateName, {
		name: "new-name",
	});
});

test("add and remove a group", async ({ page }) => {
	requiresLicense();

	await login(page, users.userAdmin);
	const orgName = defaultOrganizationName;
	const groupName = await createGroup(page, orgName);

	await login(page, users.templateAdmin);
	const templateName = await createTemplate(page);

	await page.goto(
		`/templates/${orgName}/${templateName}/settings/permissions`,
		{ waitUntil: "domcontentloaded" },
	);

	// Type the first half of the group name
	await page
		.getByPlaceholder("Search for user or group", { exact: true })
		.fill(groupName.slice(0, 4));

	// Select the group from the list and add it
	await page.getByText(groupName).click();
	await page.getByText("Add member").click();
	const row = page.locator(".MuiTableRow-root", { hasText: groupName });
	await expect(row).toBeVisible();

	// Now remove the group
	await row.getByLabel("More options").click();
	await page.getByText("Remove").click();
	await expect(page.getByText("Group removed successfully!")).toBeVisible();
	await expect(row).not.toBeVisible();
});

test("require latest version", async ({ page }) => {
	requiresLicense();

	const templateName = await createTemplate(page);

	await page.goto(`/templates/${templateName}/settings`, {
		waitUntil: "domcontentloaded",
	});
	await expectUrl(page).toHavePathName(`/templates/${templateName}/settings`);
	let checkbox = await page.waitForSelector("#require_active_version");
	await checkbox.click();
	await page.getByRole("button", { name: /save/i }).click();

	await page.goto(`/templates/${templateName}/settings`, {
		waitUntil: "domcontentloaded",
	});
	checkbox = await page.waitForSelector("#require_active_version");
	await checkbox.scrollIntoViewIfNeeded();
	expect(await checkbox.isChecked()).toBe(true);
});
