import { expect, test } from "@playwright/test";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("list templates", async ({ page, baseURL }) => {
	await page.goto(`${baseURL}/templates`, { waitUntil: "domcontentloaded" });
	await expect(page).toHaveTitle("Templates - Coder");
});
