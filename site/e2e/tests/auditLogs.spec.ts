import { test } from "@playwright/test";
import { createTemplate, createWorkspace } from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("yay audit logs", async ({ page }) => {
  const template = await createTemplate(page);
  await createWorkspace(page, template);

  page.goto("/audit");
  await page.pause();
});
