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
	test.describe.configure({ retries: 1 });

	test("show empty table when no group mappings are present", async ({
		page,
	}) => {
		requiresLicense();
		const org = await createOrganizationWithName(randomName());
		await page.goto(`/organizations/${org.name}/idp-sync?tab=groups`, {
			waitUntil: "domcontentloaded",
		});

		await expect(
			page.getByRole("row", { name: "idp-group-1" }),
		).not.toBeVisible();
		await expect(
			page.getByRole("heading", { name: "No group mappings" }),
		).toBeVisible();

		await deleteOrganization(org.name);
	});

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

		await expect(page.getByRole("row", { name: "idp-group-1" })).toBeVisible();
		await expect(
			page.getByRole("row", { name: "fbd2116a-8961-4954-87ae-e4575bd29ce0" }),
		).toBeVisible();

		await expect(page.getByRole("row", { name: "idp-group-2" })).toBeVisible();
		await expect(
			page.getByRole("row", { name: "6b39f0f1-6ad8-4981-b2fc-d52aef53ff1b" }),
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

		const row = page.getByTestId("group-idp-group-1");
		await expect(row.getByRole("cell", { name: "idp-group-1" })).toBeVisible();
		await row.getByRole("button", { name: /delete/i }).click();
		await expect(
			row.getByRole("cell", { name: "idp-group-1" }),
		).not.toBeVisible();
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
		const saveButton = page.getByRole("button", { name: /save/i });

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
		const addButton = page.getByRole("button", {
			name: /Add IdP group/i,
		});

		await expect(addButton).toBeDisabled();

		await idpOrgInput.fill("new-idp-group");

		// Select Coder organization from combobox
		const groupSelector = page.getByPlaceholder("Select group");
		await expect(groupSelector).toBeAttached();
		await expect(groupSelector).toBeVisible();
		await groupSelector.click();
		await page.waitForTimeout(1000);

		const option = await page.getByRole("option", { name: /Everyone/i });
		await expect(option).toBeAttached({ timeout: 30000 });
		await expect(option).toBeVisible();
		await option.click();

		// Add button should now be enabled
		await expect(addButton).toBeEnabled();

		await addButton.click();

		// Verify new mapping appears in table
		const newRow = page.getByTestId("group-new-idp-group");
		await expect(newRow).toBeVisible();
		await expect(
			newRow.getByRole("cell", { name: "new-idp-group" }),
		).toBeVisible();
		await expect(newRow.getByRole("cell", { name: "Everyone" })).toBeVisible();

		await expect(
			page.getByText("IdP Group sync settings updated."),
		).toBeVisible();

		await deleteOrganization(orgName);
	});
});
