import { expect, test } from "@playwright/test";
import {
	createOrganizationSyncSettings,
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

test.describe("IdpOrgSyncPage", () => {
	test.describe.configure({ retries: 1 });

	test("show empty table when no org mappings are present", async ({
		page,
	}) => {
		requiresLicense();
		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		await expect(
			page.getByRole("row", { name: "idp-org-1" }),
		).not.toBeVisible();
		await expect(
			page.getByRole("heading", { name: "No organization mappings" }),
		).toBeVisible();
	});

	test("add new IdP organization mapping with API", async ({ page }) => {
		requiresLicense();

		await createOrganizationSyncSettings();

		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		await expect(
			page.getByRole("switch", { name: "Assign Default Organization" }),
		).toBeChecked();

		await expect(page.getByRole("row", { name: "idp-org-1" })).toBeVisible();
		await expect(
			page.getByRole("row", { name: "fbd2116a-8961-4954-87ae-e4575bd29ce0" }),
		).toBeVisible();

		await expect(page.getByRole("row", { name: "idp-org-2" })).toBeVisible();
		await expect(
			page.getByRole("row", { name: "6b39f0f1-6ad8-4981-b2fc-d52aef53ff1b" }),
		).toBeVisible();
	});

	test("delete a IdP org to coder org mapping row", async ({ page }) => {
		requiresLicense();
		await createOrganizationSyncSettings();
		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		const row = page.getByTestId("idp-org-idp-org-1");
		await expect(row.getByRole("cell", { name: "idp-org-1" })).toBeVisible();
		await row.getByRole("button", { name: /delete/i }).click();
		await expect(
			row.getByRole("cell", { name: "idp-org-1" }),
		).not.toBeVisible();
		await expect(
			page.getByText("Organization sync settings updated."),
		).toBeVisible();
	});

	test("update sync field", async ({ page }) => {
		requiresLicense();
		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		const syncField = page.getByRole("textbox", {
			name: "Organization sync field",
		});
		const saveButton = page.getByRole("button", { name: /save/i });

		await expect(saveButton).toBeDisabled();

		await syncField.fill("test-field");
		await expect(saveButton).toBeEnabled();

		await page.getByRole("button", { name: /save/i }).click();

		await expect(
			page.getByText("Organization sync settings updated."),
		).toBeVisible();
	});

	test("toggle off default organization assignment", async ({ page }) => {
		requiresLicense();
		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		const toggle = page.getByRole("switch", {
			name: "Assign Default Organization",
		});
		await toggle.click();

		const dialog = page.getByRole("dialog");
		await expect(dialog).toBeVisible();

		await dialog.getByRole("button", { name: "Confirm" }).click();
		await expect(dialog).not.toBeVisible();

		await expect(
			page.getByText("Organization sync settings updated."),
		).toBeVisible();

		await expect(toggle).not.toBeChecked();
	});

	test("export policy button is enabled when sync settings are present", async ({
		page,
	}) => {
		requiresLicense();

		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		const exportButton = page.getByRole("button", { name: /Export Policy/i });
		await createOrganizationSyncSettings();

		await expect(exportButton).toBeEnabled();
		await exportButton.click();
	});

	test("add new IdP organization mapping with UI", async ({ page }) => {
		requiresLicense();

		const orgName = randomName();

		await createOrganizationWithName(orgName);

		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		const syncField = page.getByRole("textbox", {
			name: "Organization sync field",
		});
		await syncField.fill("");

		const idpOrgInput = page.getByLabel("IdP organization name");
		const addButton = page.getByRole("button", {
			name: /Add IdP organization/i,
		});

		await expect(addButton).toBeDisabled();

		const idpOrgName = randomName();
		await idpOrgInput.fill(idpOrgName);

		// Select Coder organization from combobox
		const orgSelector = page.getByPlaceholder("Select organization");
		await expect(orgSelector).toBeAttached();
		await expect(orgSelector).toBeVisible();
		await orgSelector.click();
		await page.waitForTimeout(1000);

		const option = await page.getByRole("option", { name: orgName });
		await expect(option).toBeAttached({ timeout: 30000 });
		await expect(option).toBeVisible();
		await option.click();

		// Add button should now be enabled
		await expect(addButton).toBeEnabled();

		await addButton.click();

		// Verify new mapping appears in table
		const newRow = page.getByTestId(`idp-org-${idpOrgName}`);
		await expect(newRow).toBeVisible();
		await expect(newRow.getByRole("cell", { name: idpOrgName })).toBeVisible();
		await expect(newRow.getByRole("cell", { name: orgName })).toBeVisible();

		await expect(
			page.getByText("Organization sync settings updated."),
		).toBeVisible();

		await deleteOrganization(orgName);
	});
});
