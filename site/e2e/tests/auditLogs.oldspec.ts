import { expect, test } from "@playwright/test";
import {
	createTemplate,
	createWorkspace,
	login,
	requiresLicense,
} from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("inspecting and filtering audit logs", async ({ page }) => {
	requiresLicense();

	const userName = "admin";
	// Do some stuff that should show up in the audit logs
	const templateName = await createTemplate(page);
	const workspaceName = await createWorkspace(page, templateName);

	// Go to the audit history
	await page.goto("/audit");

	const loginMessage = `${userName} logged in`;
	const startedWorkspaceMessage = `${userName} started workspace ${workspaceName}`;

	// Make sure those things we did all actually show up
	await expect(page.getByText(loginMessage).first()).toBeVisible();

	await page.getByText("All actions").click();
	await page.getByText("Create", { exact: true }).click();

	await expect(
		page.getByText(`${userName} created template ${templateName}`),
	).toBeVisible();
	await expect(
		page.getByText(`${userName} created workspace ${workspaceName}`),
	).toBeVisible();

	// Make sure we can inspect the details of the log item
	const createdWorkspace = page.locator(".MuiTableRow-root", {
		hasText: `${userName} created workspace ${workspaceName}`,
	});
	await createdWorkspace.getByLabel("open-dropdown").click();
	await expect(
		createdWorkspace.getByText(`automatic_updates: "never"`),
	).toBeVisible();
	await expect(
		createdWorkspace.getByText(`name: "${workspaceName}"`),
	).toBeVisible();
	await page.getByLabel("Clear search").click();
	await expect(page.getByText("All actions")).toBeVisible();

	await page.getByText("All actions").click();
	await page.getByText("Start", { exact: true }).click();
	await expect(page.getByText(startedWorkspaceMessage)).toBeVisible();
	await page.getByLabel("Clear search").click();
	await expect(page.getByText("All actions")).toBeVisible();

	// Filter by resource type
	await page.getByText("All resource types").click();
	const workspaceBuildsOption = page.getByText("Workspace Build");
	await workspaceBuildsOption.scrollIntoViewIfNeeded({ timeout: 5000 });
	await workspaceBuildsOption.click();
	// Our workspace build should be visible
	await expect(page.getByText(startedWorkspaceMessage)).toBeVisible();
	// Logins should no longer be visible
	await expect(page.getByText(loginMessage)).not.toBeVisible();
	await page.getByLabel("Clear search").click();
	await expect(page.getByText("All resource types")).toBeVisible();

	// Filter by action type
	await page.getByText("All actions").click();
	await page.getByText("Login").click();
	// Logins should be visible
	await expect(page.getByText(loginMessage).first()).toBeVisible();
	// Our workspace build should no longer be visible
	await expect(page.getByText(startedWorkspaceMessage)).not.toBeVisible();
});
