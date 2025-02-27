import path from "node:path";
import { expect, test } from "@playwright/test";
import {
	currentUser,
	importTemplate,
	login,
	randomName,
	requiresLicense,
} from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

// NOTE: requires the `workspace-prebuilds` experiment enabled!
test("create template with desired prebuilds", async ({ page, baseURL }) => {
	requiresLicense();

	const expectedPrebuilds = 2;

	// Create new template.
	const templateName = randomName();
	await importTemplate(page, templateName, [
		path.join(__dirname, "basic-presets-with-prebuild/main.tf"),
		path.join(__dirname, "basic-presets-with-prebuild/.terraform.lock.hcl"),
	]);

	await page.goto(
		`/workspaces?filter=owner:prebuilds%20template:${templateName}&page=1`,
		{ waitUntil: "domcontentloaded" },
	);

	// Wait for prebuilds to show up.
	const prebuilds = page.getByTestId(/^workspace-.+$/);
	await prebuilds.first().waitFor({ state: "visible", timeout: 120_000 });
	expect((await prebuilds.all()).length).toEqual(expectedPrebuilds);

	// Wait for prebuilds to start.
	const runningPrebuilds = page.getByTestId("build-status").getByText("Running");
	await runningPrebuilds.first().waitFor({ state: "visible", timeout: 120_000 });
	expect((await runningPrebuilds.all()).length).toEqual(expectedPrebuilds);
});

// NOTE: requires the `workspace-prebuilds` experiment enabled!
test("claim prebuild matching selected preset", async ({ page, baseURL }) => {
	test.setTimeout(300_000);

	requiresLicense();

	// Create new template.
	const templateName = randomName();
	await importTemplate(page, templateName, [
		path.join(__dirname, "basic-presets-with-prebuild/main.tf"),
		path.join(__dirname, "basic-presets-with-prebuild/.terraform.lock.hcl"),
	]);

	await page.goto(
		`/workspaces?filter=owner:prebuilds%20template:${templateName}&page=1`,
		{ waitUntil: "domcontentloaded" },
	);

	// Wait for prebuilds to show up.
	const prebuilds = page.getByTestId(/^workspace-.+$/);
	await prebuilds.first().waitFor({ state: "visible", timeout: 120_000 });

	// Wait for prebuilds to start.
	const runningPrebuilds = page.getByTestId("build-status").getByText("Running");
	await runningPrebuilds.first().waitFor({ state: "visible", timeout: 120_000 });

	// Open the first prebuild.
	await runningPrebuilds.first().click();
	await page.waitForURL(/\/@prebuilds\/prebuild-.+/);

	// Wait for the prebuild to become ready so it's eligible to be claimed.
	await page.getByTestId("agent-status-ready").waitFor({ timeout: 60_000 });

	// Create a new workspace using the same preset as one of the prebuilds.
	await page.goto(`/templates/coder/${templateName}/workspace`, {
		waitUntil: "domcontentloaded",
	});

	// Visit workspace creation page for new template.
	await page.goto(`/templates/default/${templateName}/workspace`, {
		waitUntil: "domcontentloaded",
	});

	// Choose a preset.
	await page.locator('button[aria-label="Preset"]').click();
	// Choose the GoLand preset.
	const preset = page.getByText("I Like GoLand");
	await expect(preset).toBeVisible();
	await preset.click();

	// Create a workspace.
	const workspaceName = randomName();
	await page.locator("input[name=name]").fill(workspaceName);
	await page.getByRole("button", { name: "Create workspace" }).click();

	// Wait for the workspace build display to be navigated to.
	const user = currentUser(page);
	await page.waitForURL(`/@${user.username}/${workspaceName}`, {
		timeout: 120_000, // Account for workspace build time.
	});

	// Validate the workspace metadata that it was indeed a claimed prebuild.
	const indicator = page.getByText("Was Prebuild");
	await indicator.waitFor({ timeout: 60_000 });
	const text = indicator.locator("xpath=..").getByText("Yes");
	await text.waitFor({ timeout: 30_000 });
});
