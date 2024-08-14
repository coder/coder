import { expect, test } from "@playwright/test";
import {
  createTemplate,
  createWorkspace,
  requiresEnterpriseLicense,
} from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(({ page }) => beforeCoderTest(page));

test("inspecting and filtering audit logs", async ({ page }) => {
  requiresEnterpriseLicense();

  const userName = "admin";
  // Do some stuff that should show up in the audit logs
  const templateName = await createTemplate(page);
  const workspaceName = await createWorkspace(page, templateName);

  // Go to the audit history
  await page.goto("/audit");

  // Make sure those things we did all actually show up
  await expect(page.getByText(`${userName} logged in`)).toBeVisible();
  await expect(
    page.getByText(`${userName} created template ${templateName}`),
  ).toBeVisible();
  await expect(
    page.getByText(`${userName} created workspace ${workspaceName}`),
  ).toBeVisible();
  await expect(
    page.getByText(`${userName} started workspace ${workspaceName}`),
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

  const startedWorkspaceMessage = `${userName} started workspace ${workspaceName}`;
  const loginMessage = `${userName} logged in`;

  // Filter by resource type
  await page.getByText("All resource types").click();
  await page.getByRole("menu").getByText("Workspace Build").click();
  // Our workspace build should be visible
  await expect(page.getByText(startedWorkspaceMessage)).toBeVisible();
  // Logins should no longer be visible
  await expect(page.getByText(loginMessage)).not.toBeVisible();

  // Clear filters, everything should be visible again
  await page.getByLabel("Clear filter").click();
  await expect(page.getByText(startedWorkspaceMessage)).toBeVisible();
  await expect(page.getByText(loginMessage)).toBeVisible();

  // Filter by action type
  await page.getByText("All actions").click();
  await page.getByRole("menu").getByText("Login").click();
  // Logins should be visible
  await expect(page.getByText(loginMessage)).toBeVisible();
  // Our workspace build should no longer be visible
  await expect(page.getByText(startedWorkspaceMessage)).not.toBeVisible();
});
