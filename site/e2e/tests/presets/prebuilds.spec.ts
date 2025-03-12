import path from "node:path";
import { type Locator, expect, test } from "@playwright/test";
import { users } from "../../constants";
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

const waitForBuildTimeout = 120_000; // Builds can take a while, let's give them at most 2m.

const templateFiles = [
	path.join(__dirname, "basic-presets-with-prebuild/main.tf"),
	path.join(__dirname, "basic-presets-with-prebuild/.terraform.lock.hcl"),
];

const expectedPrebuilds = 2;

// TODO: update provider version in *.tf

// NOTE: requires the `workspace-prebuilds` experiment enabled!
test("create template with desired prebuilds", async ({ page, baseURL }) => {
	requiresLicense();

	// Create new template.
	const templateName = randomName();
	await importTemplate(page, templateName, templateFiles);

	await page.goto(
		`/workspaces?filter=owner:prebuilds%20template:${templateName}&page=1`,
		{ waitUntil: "domcontentloaded" },
	);

	// Wait for prebuilds to show up.
	const prebuilds = page.getByTestId(/^workspace-.+$/);
	await waitForExpectedCount(prebuilds, expectedPrebuilds);

	// Wait for prebuilds to start.
	const runningPrebuilds = page
		.getByTestId("build-status")
		.getByText("Running");
	await waitForExpectedCount(runningPrebuilds, expectedPrebuilds);
});

// NOTE: requires the `workspace-prebuilds` experiment enabled!
test("claim prebuild matching selected preset", async ({ page, baseURL }) => {
	test.setTimeout(300_000);

	requiresLicense();

	// Create new template.
	const templateName = randomName();
	await importTemplate(page, templateName, templateFiles);

	await page.goto(
		`/workspaces?filter=owner:prebuilds%20template:${templateName}&page=1`,
		{ waitUntil: "domcontentloaded" },
	);

	// Wait for prebuilds to show up.
	const prebuilds = page.getByTestId(/^workspace-.+$/);
	await waitForExpectedCount(prebuilds, expectedPrebuilds);

	const previousWorkspaceNames = await Promise.all(
		(await prebuilds.all()).map((value) => {
			return value.getByText(/prebuild-.+/).textContent();
		}),
	);

	// Wait for prebuilds to start.
	let runningPrebuilds = page.getByTestId("build-status").getByText("Running");
	await waitForExpectedCount(runningPrebuilds, expectedPrebuilds);

	// Open the first prebuild.
	await runningPrebuilds.first().click();
	await page.waitForURL(/\/@prebuilds\/prebuild-.+/);

	// Wait for the prebuild to become ready so it's eligible to be claimed.
	await page.getByTestId("agent-status-ready").waitFor({ timeout: 60_000 });

	// Logout as admin, and login as an unprivileged user.
	await login(page, users.member);

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
		timeout: waitForBuildTimeout, // Account for workspace build time.
	});

	// Validate the workspace metadata that it was indeed a claimed prebuild.
	const indicator = page.getByText("Was Prebuild");
	await indicator.waitFor({ timeout: 60_000 });
	const text = indicator.locator("xpath=..").getByText("Yes");
	await text.waitFor({ timeout: 30_000 });

	// Logout as unprivileged user, and login as admin.
	await login(page, users.owner);

	// Navigate back to prebuilds page to see that a new prebuild replaced the claimed one.
	await page.goto(
		`/workspaces?filter=owner:prebuilds%20template:${templateName}&page=1`,
		{ waitUntil: "domcontentloaded" },
	);

	// Wait for prebuilds to show up.
	const newPrebuilds = page.getByTestId(/^workspace-.+$/);
	await waitForExpectedCount(newPrebuilds, expectedPrebuilds);

	const currentWorkspaceNames = await Promise.all(
		(await newPrebuilds.all()).map((value) => {
			return value.getByText(/prebuild-.+/).textContent();
		}),
	);

	// Ensure the prebuilds have changed.
	expect(currentWorkspaceNames).not.toEqual(previousWorkspaceNames);

	// Wait for prebuilds to start.
	runningPrebuilds = page.getByTestId("build-status").getByText("Running");
	await waitForExpectedCount(runningPrebuilds, expectedPrebuilds);
});

function waitForExpectedCount(prebuilds: Locator, expectedCount: number) {
	return expect
		.poll(
			async () => {
				return (await prebuilds.all()).length === expectedCount;
			},
			{
				intervals: [100],
				timeout: waitForBuildTimeout,
			},
		)
		.toBe(true);
}
