import path from "node:path";
import { expect, test } from "@playwright/test";
import {
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

	const expectedPrebuilds = 3;

	// Create new template.
	const templateName = randomName();
	await importTemplate(page, templateName, [
		path.join(__dirname, "basic-presets-with-prebuild/main.tf"),
		path.join(__dirname, "basic-presets-with-prebuild/.terraform.lock.hcl"),
	]);

	await page.goto(
		`/workspaces?filter=owner:prebuilds%20template:${templateName}&page=1`,
	);

	// Wait for prebuilds to show up.
	const prebuilds = page.getByTestId(/^workspace-.+$/);
	await prebuilds.first().waitFor({ state: "visible", timeout: 120_000 });
	expect((await prebuilds.all()).length).toEqual(expectedPrebuilds);

	// Wait for prebuilds to become ready.
	const readyPrebuilds = page.getByTestId("build-status");
	await readyPrebuilds.first().waitFor({ state: "visible", timeout: 120_000 });
	expect((await readyPrebuilds.all()).length).toEqual(expectedPrebuilds);
});
