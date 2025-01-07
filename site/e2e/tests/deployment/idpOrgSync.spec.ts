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
	test("add new IdP organization mapping with API", async ({ page }) => {
		requiresLicense();

		await createOrganizationSyncSettings();

		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		await expect(
			page.getByRole("switch", { name: "Assign Default Organization" }),
		).toBeChecked();

		await expect(page.getByText("idp-org-1")).toBeVisible();
		await expect(
			page.getByText("fbd2116a-8961-4954-87ae-e4575bd29ce0").first(),
		).toBeVisible();

		await expect(page.getByText("idp-org-2")).toBeVisible();
		await expect(
			page.getByText("fbd2116a-8961-4954-87ae-e4575bd29ce0").last(),
		).toBeVisible();
	});

	test("delete a IdP org to coder org mapping row", async ({ page }) => {
		requiresLicense();
		await createOrganizationSyncSettings();
		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		await expect(page.getByText("idp-org-1")).toBeVisible();
		await page
			.getByRole("button", { name: /delete/i })
			.first()
			.click();
		await expect(page.getByText("idp-org-1")).not.toBeVisible();
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
		const saveButton = page.getByRole("button", { name: /save/i }).first();

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

		const idpOrgInput = page.getByLabel("IdP organization name");
		const orgSelector = page.getByPlaceholder("Select organization");
		const addButton = page.getByRole("button", {
			name: /Add IdP organization/i,
		});

		await expect(addButton).toBeDisabled();

		await idpOrgInput.fill("new-idp-org");

		// Select Coder organization from combobox
		await orgSelector.click();
		await page.getByRole("option", { name: orgName }).click();

		// Add button should now be enabled
		await expect(addButton).toBeEnabled();

		await addButton.click();

		// Verify new mapping appears in table
		const newRow = page.getByTestId("idp-org-new-idp-org");
		await expect(newRow).toBeVisible();
		await expect(newRow.getByText("new-idp-org")).toBeVisible();
		await expect(newRow.getByText(orgName)).toBeVisible();

		await expect(
			page.getByText("Organization sync settings updated."),
		).toBeVisible();

		await deleteOrganization(orgName);
	});
});
