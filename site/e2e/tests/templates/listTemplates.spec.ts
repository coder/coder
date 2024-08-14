import { test, expect } from "@playwright/test";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(({ page }) => beforeCoderTest(page));

test("list templates", async ({ page, baseURL }) => {
  await page.goto(`${baseURL}/templates`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Templates - Coder");
});
