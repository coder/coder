import path from "node:path";
import { expect, test } from "@playwright/test";
import { currentUser, importTemplate, login, randomName } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("create template with preset and use in workspace", async ({
	page,
	baseURL,
}) => {
	// Create new template.
	const templateName = randomName();
	await importTemplate(page, templateName, [
		path.join(__dirname, "basic-presets/main.tf"),
		path.join(__dirname, "basic-presets/.terraform.lock.hcl"),
	]);

	// Visit workspace creation page for new template.
	await page.goto(`/templates/default/${templateName}/workspace`, {
		waitUntil: "domcontentloaded",
	});

	await page.locator('button[aria-label="Preset"]').click();

	const preset1 = page.getByText("I Like GoLand");
	const preset2 = page.getByText("Some Like PyCharm");

	await expect(preset1).toBeVisible();
	await expect(preset2).toBeVisible();

	// Choose the GoLand preset.
	await preset1.click();

	// Validate the preset was applied correctly.
	await expect(page.locator('input[value="GO"]')).toBeChecked();

	// Create a workspace.
	const workspaceName = randomName();
	await page.locator("input[name=name]").fill(workspaceName);
	await page.getByRole("button", { name: "Create workspace" }).click();

	// Wait for the workspace build display to be navigated to.
	const user = currentUser(page);
	await page.waitForURL(`/@${user.username}/${workspaceName}`, {
		timeout: 120_000, // Account for workspace build time.
	});

	// Visit workspace settings page.
	await page.goto(`/@${user.username}/${workspaceName}/settings/parameters`);

	// Validate the preset was applied correctly.
	await expect(page.locator('input[value="GO"]')).toBeChecked();
});
