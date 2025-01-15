import { expect, test } from "@playwright/test";
import {
	createGroupSyncSettings,
	createOrganizationWithName,
	deleteOrganization,
	setupApiCalls,
} from "../../api";
import { randomName, requiresLicense } from "../../helpers";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test.describe("IdpGroupSyncPage", () => {
	test("add new IdP group mapping with API", async ({ page }) => {
		requiresLicense();
		const org = await createOrganizationWithName(randomName());
		await createGroupSyncSettings(org.id);

		await page.goto(`/organizations/${org.name}/idp-sync?tab=groups`, {
			waitUntil: "domcontentloaded",
		});

		await expect(
			page.getByRole("switch", { name: "Auto create missing groups" }),
		).toBeChecked();

		await expect(page.getByText("idp-group-1")).toBeVisible();
		await expect(
			page.getByText("fbd2116a-8961-4954-87ae-e4575bd29ce0").first(),
		).toBeVisible();

		await expect(page.getByText("idp-group-2")).toBeVisible();
		await expect(
			page.getByText("fbd2116a-8961-4954-87ae-e4575bd29ce0").last(),
		).toBeVisible();

		await deleteOrganization(org.name);
	});

	test("delete a IdP group to coder group mapping row", async ({ page }) => {
		requiresLicense();
		const org = await createOrganizationWithName(randomName());
		await createGroupSyncSettings(org.id);

		await page.goto(`/organizations/${org.name}/idp-sync?tab=groups`, {
			waitUntil: "domcontentloaded",
		});

		await expect(page.getByText("idp-group-1")).toBeVisible();
		await page
			.getByRole("button", { name: /delete/i })
			.first()
			.click();
		await expect(page.getByText("idp-group-1")).not.toBeVisible();
		await expect(
			page.getByText("IdP Group sync settings updated."),
		).toBeVisible();
	});

	test("update sync field", async ({ page }) => {
		requiresLicense();
		const org = await createOrganizationWithName(randomName());
		await page.goto(`/organizations/${org.name}/idp-sync?tab=groups`, {
			waitUntil: "domcontentloaded",
		});

		const syncField = page.getByRole("textbox", {
			name: "Group sync field",
		});
		const saveButton = page.getByRole("button", { name: /save/i }).first();

		await expect(saveButton).toBeDisabled();

		await syncField.fill("test-field");
		await expect(saveButton).toBeEnabled();

		await page.getByRole("button", { name: /save/i }).click();

		await expect(
			page.getByText("IdP Group sync settings updated."),
		).toBeVisible();
	});

	test("toggle off auto create missing groups", async ({ page }) => {
		requiresLicense();
		const org = await createOrganizationWithName(randomName());
		await page.goto(`/organizations/${org.name}/idp-sync?tab=groups`, {
			waitUntil: "domcontentloaded",
		});

		const toggle = page.getByRole("switch", {
			name: "Auto create missing groups",
		});
		await toggle.click();

		await expect(
			page.getByText("IdP Group sync settings updated."),
		).toBeVisible();

		await expect(toggle).toBeChecked();
	});

	test("export policy button is enabled when sync settings are present", async ({
		page,
	}) => {
		requiresLicense();
		const org = await createOrganizationWithName(randomName());
		await createGroupSyncSettings(org.id);
		await page.goto(`/organizations/${org.name}/idp-sync?tab=groups`, {
			waitUntil: "domcontentloaded",
		});

		const exportButton = page.getByRole("button", { name: /Export Policy/i });
		await expect(exportButton).toBeEnabled();
		await exportButton.click();
	});

	test("add new IdP group mapping with UI", async ({ page }) => {
		requiresLicense();
		const orgName = randomName();
		await createOrganizationWithName(orgName);

		await page.goto(`/organizations/${orgName}/idp-sync?tab=groups`, {
			waitUntil: "domcontentloaded",
		});

		const idpOrgInput = page.getByLabel("IdP group name");
		const orgSelector = page.getByPlaceholder("Select group");
		const addButton = page.getByRole("button", {
			name: /Add IdP group/i,
		});

		await expect(addButton).toBeDisabled();

		await idpOrgInput.fill("new-idp-group");

		// Select Coder organization from combobox
		await orgSelector.click();
		await page.getByRole("option", { name: /Everyone/i }).click();

		// Add button should now be enabled
		await expect(addButton).toBeEnabled();

		await addButton.click();

		// Verify new mapping appears in table
		const newRow = page.getByTestId("group-new-idp-group");
		await expect(newRow).toBeVisible();
		await expect(newRow.getByText("new-idp-group")).toBeVisible();
		await expect(newRow.getByText("Everyone")).toBeVisible();

		await expect(
			page.getByText("IdP Group sync settings updated."),
		).toBeVisible();

		await deleteOrganization(orgName);
	});
});
