import { expect, test } from "@playwright/test";
import {
	createOrganizationSyncSettings,
	createOrganizationWithName,
	deleteOrganization,
	setupApiCalls,
} from "../../api";
import { requiresLicense } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.describe("IdpOrgSyncPage", () => {
	test.beforeEach(async ({ page }) => await beforeCoderTest(page));

	test("add new IdP organization mapping with API", async ({ page }) => {
		requiresLicense();
		await setupApiCalls(page);

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
		await setupApiCalls(page);
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
		const saveButton = page.getByRole("button", { name: "Save" }).first();

		await expect(saveButton).toBeDisabled();

		await syncField.fill("test-field");
		await expect(saveButton).toBeEnabled();

		await page.getByRole("button", { name: "Save" }).click();

		await expect(
			page.getByText("Organization sync settings updated."),
		).toBeVisible();
	});

	test("toggle default organization assignment", async ({ page }) => {
		requiresLicense();
		await page.goto("/deployment/idp-org-sync", {
			waitUntil: "domcontentloaded",
		});

		const toggle = page.getByRole("switch", {
			name: "Assign Default Organization",
		});
		await toggle.click();

		await expect(
			page.getByText("Organization sync settings updated."),
		).toBeVisible();

		await expect(toggle).not.toBeChecked();
	});

	test("export policy button is enabled when sync settings are present", async ({
		page,
	}) => {
		requiresLicense();
		await setupApiCalls(page);

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
		await setupApiCalls(page);

		await createOrganizationWithName("developers");

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
		await page.getByRole("option", { name: "developers" }).click();

		// Add button should now be enabled
		await expect(addButton).toBeEnabled();

		await addButton.click();

		// Verify new mapping appears in table
		const newRow = page.getByTestId("idp-org-new-idp-org");
		await expect(newRow).toBeVisible();
		await expect(newRow.getByText("new-idp-org")).toBeVisible();
		await expect(newRow.getByText("developers")).toBeVisible();

		await expect(
			page.getByText("Organization sync settings updated."),
		).toBeVisible();

		await deleteOrganization("developers");
	});
});
