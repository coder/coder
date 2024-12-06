import { test, expect } from "@playwright/test";
import { MockOrganization } from "testHelpers/entities";
import {
	createOrganization,
	createCustomRole,
	getCurrentOrgId,
	setupApiCalls,
} from "../../../api";
import { requiresLicense } from "../../../helpers";
import { beforeCoderTest } from "../../../hooks";

test.describe("CustomRolesPage", () => {

  test.beforeEach(async ({ page }) => await beforeCoderTest(page));

  test("create custom role", async ({ page }) => {
	requiresLicense();
	await setupApiCalls(page);

	const org = await createOrganization();
	const customRole = await createCustomRole(org.id, "custom-role-test-1", "Custom Role Test 1");

	await page.goto(`/organizations/${org.name}/roles`);
	const roleRow = page.getByTestId(`role-${customRole.name}`);
	await expect(roleRow.getByText(customRole.display_name)).toBeVisible();
	await expect(roleRow.getByText("organization_member")).toBeVisible();

  });

  test("displays built-in role without edit/delete options", async ({ page }) => {
	requiresLicense();
	await setupApiCalls(page);

	const org = await createOrganization();
	await page.goto(`/organizations/${org.name}/roles`);

    const roleRow = page.getByTestId("role-organization-admin");
    await expect(roleRow).toBeVisible();

    await expect(roleRow.getByText("Organization Admin")).toBeVisible();

    // Verify that the more menu (three dots) is not present for built-in roles
    await expect(roleRow.getByRole("button", { name: "More options" })).not.toBeVisible();
  });

  test("can navigate to create custom role", async ({ page }) => {
	requiresLicense();
	await setupApiCalls(page);

	const org = await createOrganization();
	await page.goto(`/organizations/${org.name}/roles`);

    await page.getByRole("link", { name: "Create custom role" }).first().click();
    await expect(page).toHaveURL(`/organizations/${org.name}/roles/create`);
  });

//   test("delete custom role", async ({ page }) => {
// 	requiresLicense();
// 	await setupApiCalls(page);

// 	const org = await createOrganization();
// 	const customRole = await createCustomRole(org.id, "custom-role-test-1", "Custom Role Test 1");
// 	await page.goto(`/organizations/${org.name}/roles`);

//     // const roleRow = page.getByTestId("role-custom-role-test-1");
//     await page.getByRole("button", { name: "More options" }).click();

//     // Check menu items
//     await expect(page.getByRole("menuitem", { name: "Edit" })).toBeVisible();
// 	await expect(page.getByText("Edit")).toBeVisible();
// 	// const menu = page.getByRole("menu");

// 	const deleteButton = page.getByRole("menuitem", { name: "Delete&hellip;" });
//     await expect(deleteButton).toBeVisible();
// 	await deleteButton.click();

// 	const input = page.getByRole("textbox");
// 	await input.fill(customRole.name);
// 	await page.getByRole("button", { name: "Delete" }).click();

// 	await expect(page.getByText("Custom role deleted successfully!")).toBeVisible();
//   });

//   test("shows delete confirmation dialog", async ({ page }) => {
//     // Click delete option
//     const roleRow = page.getByTestId("role-custom-role-test-1");
//     await roleRow.getByRole("button", { name: "More options" }).click();
//     await page.getByRole("menuitem", { name: "Delete…" }).click();

//     // Check dialog content
//     await expect(page.getByRole("dialog")).toBeVisible();
//     await expect(page.getByText(/Are you sure you want to delete/)).toBeVisible();
//     await expect(page.getByRole("button", { name: "Cancel" })).toBeVisible();
//     await expect(page.getByRole("button", { name: "Delete" })).toBeVisible();
//   });

//   test("handles delete role successfully", async ({ page }) => {
//     // Mock delete API call
//     await page.route("**/api/v2/organizations/*/roles/*", (route) =>
//       route.fulfill(createMockApiResponse({}))
//     );

//     // Perform delete
//     const roleRow = page.getByTestId("role-custom-role-test-1");
//     await roleRow.getByRole("button", { name: "More options" }).click();
//     await page.getByRole("menuitem", { name: "Delete…" }).click();
//     await page.getByRole("button", { name: "Delete" }).click();

//     // Check success message
//     await expect(page.getByText("Custom role deleted successfully!")).toBeVisible();
//   });

//   test("shows paywall when custom roles not enabled", async ({ page }) => {
//     // Mock feature flags to disable custom roles
//     await page.route("**/api/v2/features", (route) =>
//       route.fulfill(createMockApiResponse({
//         custom_roles: false
//       }))
//     );

//     await page.reload();

//     // Check paywall content
//     await expect(page.getByText("Upgrade to a premium license to create a custom role")).toBeVisible();
//     await expect(page.getByRole("link", { name: "Create custom role" })).not.toBeVisible();
//   });
});
