import { expect, test } from "@playwright/test";
import {
	createOrganizationWithName,
	createRoleSyncSettings,
	deleteOrganization,
	setupApiCalls,
} from "../../api";
import { randomName, requiresLicense } from "../../helpers";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	requiresLicense();
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test.describe("IdpRoleSyncPage", () => {
	test.describe.configure({ retries: 1 });

	test("show empty table when no role mappings are present", async ({
		page,
	}) => {
		const org = await createOrganizationWithName(randomName());
		await page.goto(`/organizations/${org.name}/idp-sync?tab=roles`, {
			waitUntil: "domcontentloaded",
		});

		await expect(
			page.getByRole("row", { name: "idp-role-1" }),
		).not.toBeVisible();
		await expect(
			page.getByRole("heading", { name: "No role mappings" }),
		).toBeVisible();

		await deleteOrganization(org.name);
	});

	test("add new IdP role mapping with API", async ({ page }) => {
		const org = await createOrganizationWithName(randomName());
		await createRoleSyncSettings(org.id);

		await page.goto(`/organizations/${org.name}/idp-sync?tab=roles`, {
			waitUntil: "domcontentloaded",
		});

		await expect(page.getByRole("row", { name: "idp-role-1" })).toBeVisible();
		await expect(
			page.getByRole("row", { name: "fbd2116a-8961-4954-87ae-e4575bd29ce0" }),
		).toBeVisible();

		await expect(page.getByRole("row", { name: "idp-role-2" })).toBeVisible();
		await expect(
			page.getByRole("row", { name: "fbd2116a-8961-4954-87ae-e4575bd29ce0" }),
		).toBeVisible();

		await deleteOrganization(org.name);
	});

	test("delete a IdP role to coder role mapping row", async ({ page }) => {
		const org = await createOrganizationWithName(randomName());
		await createRoleSyncSettings(org.id);

		await page.goto(`/organizations/${org.name}/idp-sync?tab=roles`, {
			waitUntil: "domcontentloaded",
		});
		const row = page.getByTestId("role-idp-role-1");
		await expect(row.getByRole("cell", { name: "idp-role-1" })).toBeVisible();
		await row.getByRole("button", { name: /delete/i }).click();
		await expect(
			row.getByRole("cell", { name: "idp-role-1" }),
		).not.toBeVisible();
		await expect(
			page.getByText("IdP Role sync settings updated."),
		).toBeVisible();

		await deleteOrganization(org.name);
	});

	test("update sync field", async ({ page }) => {
		const org = await createOrganizationWithName(randomName());
		await page.goto(`/organizations/${org.name}/idp-sync?tab=roles`, {
			waitUntil: "domcontentloaded",
		});

		const syncField = page.getByRole("textbox", {
			name: "Role sync field",
		});
		const saveButton = page.getByRole("button", { name: /save/i });

		await expect(saveButton).toBeDisabled();

		await syncField.fill("test-field");
		await expect(saveButton).toBeEnabled();

		await page.getByRole("button", { name: /save/i }).click();

		await expect(
			page.getByText("IdP Role sync settings updated."),
		).toBeVisible();

		await deleteOrganization(org.name);
	});

	test("export policy button is enabled when sync settings are present", async ({
		page,
	}) => {
		const org = await createOrganizationWithName(randomName());
		await page.goto(`/organizations/${org.name}/idp-sync?tab=roles`, {
			waitUntil: "domcontentloaded",
		});

		const exportButton = page.getByRole("button", { name: /Export Policy/i });
		await createRoleSyncSettings(org.id);

		await expect(exportButton).toBeEnabled();
		await exportButton.click();
	});

	test("add new IdP role mapping with UI", async ({ page }) => {
		const orgName = randomName();
		await createOrganizationWithName(orgName);

		await page.goto(`/organizations/${orgName}/idp-sync?tab=roles`, {
			waitUntil: "domcontentloaded",
		});

		const idpOrgInput = page.getByLabel("IdP role name");
		const addButton = page.getByRole("button", {
			name: /Add IdP role/i,
		});

		await expect(addButton).toBeDisabled();

		const idpRoleName = randomName();
		await idpOrgInput.fill(idpRoleName);

		// Select Coder role from combobox
		const roleSelector = page.getByPlaceholder("Select role");
		await expect(roleSelector).toBeAttached();
		await expect(roleSelector).toBeVisible();
		await roleSelector.click();

		await page.getByRole("combobox").click();
		await page.waitForTimeout(1000);

		const option = await page.getByRole("option", {
			name: /Organization Admin/i,
		});

		await expect(option).toBeAttached({ timeout: 30000 });
		await expect(option).toBeVisible();
		await option.click();

		// Add button should now be enabled
		await expect(addButton).toBeEnabled();

		await addButton.click();

		// Verify new mapping appears in table
		const newRow = page.getByTestId(`role-${idpRoleName}`);
		await expect(newRow).toBeVisible();
		await expect(newRow.getByRole("cell", { name: idpRoleName })).toBeVisible();
		await expect(
			newRow.getByRole("cell", { name: "organization-admin" }),
		).toBeVisible();

		await expect(
			page.getByText("IdP Role sync settings updated."),
		).toBeVisible();

		await deleteOrganization(orgName);
	});
});
