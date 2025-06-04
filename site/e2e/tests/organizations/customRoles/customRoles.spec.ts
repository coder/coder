import { expect, test } from "@playwright/test";
import {
	createCustomRole,
	createOrganizationWithName,
	deleteOrganization,
	setupApiCalls,
} from "../../../api";
import {
	login,
	randomName,
	requiresLicense,
	requiresUnlicensed,
} from "../../../helpers";
import { beforeCoderTest } from "../../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test.describe("CustomRolesPage", () => {
	requiresLicense();

	test("create custom role and cancel edit changes", async ({ page }) => {
		await setupApiCalls(page);

		const org = await createOrganizationWithName(randomName());

		const customRole = await createCustomRole(
			org.id,
			"custom-role-test-1",
			"Custom Role Test 1",
		);

		await page.goto(`/organizations/${org.name}/roles`);
		const roleRow = page.getByTestId(`role-${customRole.name}`);
		await expect(roleRow.getByText(customRole.display_name)).toBeVisible();
		await expect(roleRow.getByText("organization_member")).toBeVisible();

		await roleRow.getByRole("button", { name: "Open menu" }).click();
		const menu = page.getByRole("menu");
		await menu.getByText("Edit").click();

		await expect(page).toHaveURL(
			`/organizations/${org.name}/roles/${customRole.name}`,
		);

		const cancelButton = page.getByRole("button", { name: "Cancel" }).first();
		await expect(cancelButton).toBeVisible();
		await cancelButton.click();

		await expect(page).toHaveURL(`/organizations/${org.name}/roles`);

		await deleteOrganization(org.name);
	});

	test("create custom role, edit role and save changes", async ({ page }) => {
		await setupApiCalls(page);

		const org = await createOrganizationWithName(randomName());

		const customRole = await createCustomRole(
			org.id,
			"custom-role-test-1",
			"Custom Role Test 1",
		);

		await page.goto(`/organizations/${org.name}/roles`);
		const roleRow = page.getByTestId(`role-${customRole.name}`);
		await expect(roleRow.getByText(customRole.display_name)).toBeVisible();
		await expect(roleRow.getByText("organization_member")).toBeVisible();

		await page.goto(`/organizations/${org.name}/roles/${customRole.name}`);

		const displayNameInput = page.getByRole("textbox", {
			name: "Display name",
		});
		await displayNameInput.fill("Custom Role Test 2 Edited");

		const groupCheckbox = page.getByTestId("group").getByRole("checkbox");
		await expect(groupCheckbox).toBeVisible();
		await groupCheckbox.click();

		const organizationMemberCheckbox = page
			.getByTestId("organization_member")
			.getByRole("checkbox");
		await expect(organizationMemberCheckbox).toBeVisible();
		await organizationMemberCheckbox.click();

		const saveButton = page.getByRole("button", { name: /save/i }).first();
		await expect(saveButton).toBeVisible();
		await saveButton.click();

		await expect(roleRow.getByText("Custom Role Test 2 Edited")).toBeVisible();

		const roleRow2 = page.getByTestId(`role-${customRole.name}`);
		await expect(roleRow2.getByText("organization_member")).not.toBeVisible();
		await expect(roleRow2.getByText("group")).toBeVisible();

		await expect(page).toHaveURL(`/organizations/${org.name}/roles`);

		await deleteOrganization(org.name);
	});

	test("displays built-in role without edit/delete options", async ({
		page,
	}) => {
		await setupApiCalls(page);

		const org = await createOrganizationWithName(randomName());

		await page.goto(`/organizations/${org.name}/roles`);

		const roleRow = page.getByTestId("role-organization-admin");
		await expect(roleRow).toBeVisible();

		await expect(roleRow.getByText("Organization Admin")).toBeVisible();

		// Verify that the more menu (three dots) is not present for built-in roles
		await expect(
			roleRow.getByRole("button", { name: "Open menu" }),
		).not.toBeVisible();

		await deleteOrganization(org.name);
	});

	test("create custom role with UI", async ({ page }) => {
		await setupApiCalls(page);

		const org = await createOrganizationWithName(randomName());

		await page.goto(`/organizations/${org.name}/roles`);

		await page
			.getByRole("link", { name: "Create custom role" })
			.first()
			.click();

		await expect(page).toHaveURL(`/organizations/${org.name}/roles/create`);

		const customRoleName = "custom-role-test";
		const roleNameInput = page.getByRole("textbox", {
			exact: true,
			name: "Name",
		});
		await roleNameInput.fill(customRoleName);

		const customRoleDisplayName = "Custom Role Test";
		const displayNameInput = page.getByRole("textbox", {
			exact: true,
			name: "Display Name",
		});
		await displayNameInput.fill(customRoleDisplayName);

		await page.getByRole("button", { name: "Create Role" }).first().click();

		await expect(page).toHaveURL(`/organizations/${org.name}/roles`);

		const roleRow = page.getByTestId(`role-${customRoleName}`);
		await expect(roleRow.getByText(customRoleDisplayName)).toBeVisible();
		await expect(roleRow.getByText("None")).toBeVisible();

		await deleteOrganization(org.name);
	});

	test("delete custom role", async ({ page }) => {
		await setupApiCalls(page);

		const org = await createOrganizationWithName(randomName());
		const customRole = await createCustomRole(
			org.id,
			"custom-role-test-1",
			"Custom Role Test 1",
		);
		await page.goto(`/organizations/${org.name}/roles`);

		const roleRow = page.getByTestId(`role-${customRole.name}`);
		await roleRow.getByRole("button", { name: "Open menu" }).click();

		const menu = page.getByRole("menu");
		await menu.getByText("Delete…").click();

		const input = page.getByRole("textbox");
		await input.fill(customRole.name);
		await page.getByRole("button", { name: "Delete" }).click();

		await expect(
			page.getByText("Custom role deleted successfully!"),
		).toBeVisible();

		await deleteOrganization(org.name);
	});
});

test("custom roles disabled", async ({ page }) => {
	requiresUnlicensed();
	await page.goto("/organizations/coder/roles");
	await expect(page).toHaveURL("/organizations/coder/roles");

	await expect(
		page.getByText("Upgrade to a premium license to create a custom role"),
	).toBeVisible();
	await expect(
		page.getByRole("link", { name: "Create custom role" }),
	).not.toBeVisible();
});
