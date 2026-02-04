import { expect, test } from "@playwright/test";
import { API } from "api/api";
import { Language } from "pages/CreateUserPage/Language";
import { coderPort, license, premiumTestsRequired, users } from "../constants";
import { expectUrl } from "../expectUrl";
import { getCurrentOrgId, setupApiCalls } from "../api";
import { createTemplate, createUser, login } from "../helpers";

const waitForTemplateImport = async (versionId: string) => {
	const maxAttempts = 30;
	for (let attempt = 0; attempt < maxAttempts; attempt++) {
		const version = await API.getTemplateVersion(versionId);
		if (version.job.status === "succeeded") {
			return;
		}
		if (version.job.status === "failed" || version.job.status === "canceled") {
			throw new Error(
				`Template version ${version.name} failed to import: ${version.job.error ?? "unknown error"}`,
			);
		}
		await new Promise((resolve) => setTimeout(resolve, 1000));
	}
	throw new Error("Template version did not finish importing in time.");
};

test("setup deployment", async ({ page }) => {
	await page.goto("/", { waitUntil: "domcontentloaded" });
	API.setHost(`http://127.0.0.1:${coderPort}`);
	const exists = await API.hasFirstUser();
	// First user already exists, abort early. All tests execute this as a dependency,
	// if you run multiple tests in the UI, this will fail unless we check this.
	if (exists) {
		await login(page, users.owner);
	} else {
		// Setup first user
		await page.getByLabel(Language.emailLabel).fill(users.owner.email);
		await page.getByLabel(Language.passwordLabel).fill(users.owner.password);
		await page.getByTestId("create").click();

		await expectUrl(page).toHavePathName("/templates");
		await page.getByTestId("button-select-template").isVisible();

		for (const user of Object.values(users)) {
			// Already created as first user
			if (user.username === "owner") {
				continue;
			}

			await createUser(page, user);
		}

		// Setup license
		if (premiumTestsRequired || license) {
			// Make sure that we have something that looks like a real license
			expect(license).toBeTruthy();
			expect(license.length).toBeGreaterThan(92); // the signature alone should be this long
			expect(license.split(".").length).toBe(3); // otherwise it's invalid

			await page.goto("/deployment/licenses", { waitUntil: "domcontentloaded" });
			await expect(page).toHaveTitle("License Settings - Coder");

			await page.getByText("Add a license").click();
			await page.getByRole("textbox").fill(license);
			await page.getByText("Upload License").click();

			await expect(
				page.getByText("You have successfully added a license"),
			).toBeVisible();
		}
	}

	await setupApiCalls(page);
	const orgId = await getCurrentOrgId();
	const templates = await API.getTemplatesByOrganization(orgId);
	let hasReadyTemplate = false;
	for (const template of templates) {
		try {
			const version = await API.getTemplateVersion(template.active_version_id);
			if (version.job.status === "succeeded") {
				hasReadyTemplate = true;
				break;
			}
		} catch {
			// Ignore templates with missing versions.
		}
	}

	if (!hasReadyTemplate) {
		const templateName = await createTemplate(page);
		const template = await API.getTemplateByName(orgId, templateName);
		await waitForTemplateImport(template.active_version_id);
	}
});
