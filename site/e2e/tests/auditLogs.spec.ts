import { type Page, expect, test } from "@playwright/test";
import { users } from "../constants";
import {
	createTemplate,
	createWorkspace,
	currentUser,
	login,
	requiresLicense,
} from "../helpers";
import { beforeCoderTest } from "../hooks";

test.describe.configure({ mode: "parallel" });

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page, users.auditor);
});

async function resetSearch(page: Page) {
	const clearButton = page.getByLabel("Clear search");
	if (await clearButton.isVisible()) {
		await clearButton.click();
	}

	// Filter by the auditor test user to prevent race conditions
	const user = currentUser(page);
	await expect(page.getByText("All users")).toBeVisible();
	await page.getByPlaceholder("Search...").fill(`username:${user.username}`);
	await expect(page.getByText("All users")).not.toBeVisible();
}

test("logins are logged", async ({ page }) => {
	requiresLicense();

	// Go to the audit history
	await page.goto("/audit");

	const user = currentUser(page);
	const loginMessage = `${user.username} logged in`;
	// Make sure those things we did all actually show up
	await resetSearch(page);
	await expect(page.getByText(loginMessage).first()).toBeVisible();
});

test("creating templates and workspaces is logged", async ({ page }) => {
	requiresLicense();

	// Do some stuff that should show up in the audit logs
	const templateName = await createTemplate(page);
	const workspaceName = await createWorkspace(page, templateName);

	// Go to the audit history
	await page.goto("/audit");

	const user = currentUser(page);

	// Make sure those things we did all actually show up
	await resetSearch(page);
	await expect(
		page.getByText(`${user.username} created template ${templateName}`),
	).toBeVisible();
	await expect(
		page.getByText(`${user.username} created workspace ${workspaceName}`),
	).toBeVisible();
	await expect(
		page.getByText(`${user.username} started workspace ${workspaceName}`),
	).toBeVisible();

	// Make sure we can inspect the details of the log item
	const createdWorkspace = page.locator(".MuiTableRow-root", {
		hasText: `${user.username} created workspace ${workspaceName}`,
	});
	await createdWorkspace.getByLabel("open-dropdown").click();
	await expect(
		createdWorkspace.getByText(`automatic_updates: "never"`),
	).toBeVisible();
	await expect(
		createdWorkspace.getByText(`name: "${workspaceName}"`),
	).toBeVisible();
});

test("inspecting and filtering audit logs", async ({ page }) => {
	requiresLicense();

	// Do some stuff that should show up in the audit logs
	const templateName = await createTemplate(page);
	const workspaceName = await createWorkspace(page, templateName);

	// Go to the audit history
	await page.goto("/audit");

	const user = currentUser(page);
	const loginMessage = `${user.username} logged in`;
	const startedWorkspaceMessage = `${user.username} started workspace ${workspaceName}`;

	// Filter by resource type
	await resetSearch(page);
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
	await resetSearch(page);
	await page.getByText("All actions").click();
	await page.getByText("Login", { exact: true }).click();
	// Logins should be visible
	await expect(page.getByText(loginMessage).first()).toBeVisible();
	// Our workspace build should no longer be visible
	await expect(page.getByText(startedWorkspaceMessage)).not.toBeVisible();
});
