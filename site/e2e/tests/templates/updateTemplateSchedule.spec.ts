import { expect, test } from "@playwright/test";
import { API } from "#/api/api";
import { getCurrentOrgId, setupApiCalls } from "../../api";
import { users } from "../../constants";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page, users.templateAdmin);
	await setupApiCalls(page);
});

test("update template schedule settings without override other settings", async ({
	page,
	baseURL,
}) => {
	const orgId = await getCurrentOrgId();
	const templateVersion = await API.createTemplateVersion(orgId, {
		storage_method: "file" as const,
		provisioner: "echo",
		user_variable_values: [],
		example_id: "docker",
		tags: {},
	});
	const template = await API.createTemplate(orgId, {
		name: "test-template",
		display_name: "Test Template",
		template_version_id: templateVersion.id,
		disable_everyone_group_access: false,
		require_active_version: true,
		max_port_share_level: null,
		cors_behavior: null,
		allow_user_cancel_workspace_jobs: null,
	});

	await page.goto(`${baseURL}/templates/${template.name}/settings/schedule`, {
		waitUntil: "domcontentloaded",
	});
	for (const { defaultHours, bumpHours } of [
		{ defaultHours: 48, bumpHours: 2 },
		{ defaultHours: 0, bumpHours: 0 },
	]) {
		await test.step(`persist autostop ${defaultHours}h and activity bump ${bumpHours}h`, async () => {
			// Clear the bump before disabling autostop, which can disable its input.
			await page.getByLabel("Activity bump (hours)").fill(String(bumpHours));
			await page
				.getByLabel("Default autostop (hours)")
				.fill(String(defaultHours));
			const saved = page.waitForResponse(
				(response) =>
					response.url().endsWith(`/api/v2/templates/${template.id}`) &&
					response.request().method() === "PATCH",
			);
			await page.getByRole("button", { name: /save/i }).click();
			expect((await saved).ok()).toBe(true);

			const updatedTemplate = await API.getTemplate(template.id);
			expect(updatedTemplate).toStrictEqual({
				...template,
				default_ttl_ms: defaultHours * 60 * 60 * 1000,
				activity_bump_ms: bumpHours * 60 * 60 * 1000,
				updated_at: updatedTemplate.updated_at,
			});
			await page.reload({ waitUntil: "domcontentloaded" });
		});
	}
});
