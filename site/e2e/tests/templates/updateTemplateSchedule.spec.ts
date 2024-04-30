import { expect, test } from "@playwright/test";
import { createTemplate, createTemplateVersion, getTemplate } from "api/api";
import { getCurrentOrgId, setupApiCalls } from "../../api";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(({ page }) => beforeCoderTest(page));

test("update template schedule settings without override other settings", async ({
  page,
  baseURL,
}) => {
  await setupApiCalls(page);
  const orgId = await getCurrentOrgId();
  const templateVersion = await createTemplateVersion(orgId, {
    storage_method: "file" as const,
    provisioner: "echo",
    user_variable_values: [],
    example_id: "docker",
    tags: {},
  });
  const template = await createTemplate(orgId, {
    name: "test-template",
    display_name: "Test Template",
    template_version_id: templateVersion.id,
    disable_everyone_group_access: false,
    require_active_version: true,
  });

  await page.goto(`${baseURL}/templates/${template.name}/settings/schedule`, {
    waitUntil: "domcontentloaded",
  });
  await page.getByLabel("Default autostop (hours)").fill("48");
  await page.getByRole("button", { name: "Submit" }).click();
  await expect(page.getByText("Template updated successfully")).toBeVisible();

  const updatedTemplate = await getTemplate(template.id);
  // Validate that the template data remains consistent, with the exception of
  // the 'default_ttl_ms' field (updated during the test) and the 'updated at'
  // field (automatically updated by the backend).
  expect({
    ...template,
    default_ttl_ms: 48 * 60 * 60 * 1000,
    updated_at: updatedTemplate.updated_at,
  }).toStrictEqual(updatedTemplate);
});
