import { expect, test } from "@playwright/test";
import { API } from "api/api";
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
		allow_user_cancel_workspace_jobs: null,
	});

	await page.goto(`${baseURL}/templates/${template.name}/settings/schedule`, {
		waitUntil: "domcontentloaded",
	});
	await page.getByLabel("Default autostop (hours)").fill("48");
	await page.getByRole("button", { name: /save/i }).click();
	await expect(page.getByText("Template updated successfully")).toBeVisible();

	const updatedTemplate = await API.getTemplate(template.id);
	// Validate that the template data remains consistent, with the exception of
	// the 'default_ttl_ms' field (updated during the test) and the 'updated at'
	// field (automatically updated by the backend).
	expect({
		...template,
		default_ttl_ms: 48 * 60 * 60 * 1000,
		updated_at: updatedTemplate.updated_at,
	}).toStrictEqual(updatedTemplate);
});
