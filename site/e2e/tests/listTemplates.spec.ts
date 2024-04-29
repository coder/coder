import { beforeCoderTest } from "../hooks";
import { expect, test } from "../testing";

test.beforeEach(({ page }) => beforeCoderTest(page));

test("list templates", async ({ page, baseURL }) => {
  await page.goto(`${baseURL}/templates`, { waitUntil: "domcontentloaded" });
  await expect(page).toHaveTitle("Templates - Coder");
});
