import { expect, test } from "@playwright/test";
import { users } from "../../constants";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(({ page }) => {
	beforeCoderTest(page);
});

test("adjust user theme preference", async ({ page }) => {
	await login(page, users.member);

	await page.goto("/settings/appearance", { waitUntil: "domcontentloaded" });

	await page.getByText("Light", { exact: true }).click();
	await expect(page.getByLabel("Light")).toBeChecked();

	// Make sure the page is actually updated to use the light theme
	const [root] = await page.$$("html");
	expect(await root.evaluate((it) => it.className)).toContain("light");

	await page.goto("/", { waitUntil: "domcontentloaded" });

	// Make sure the page is still using the light theme after reloading and
	// navigating away from the settings page.
	const [homeRoot] = await page.$$("html");
	expect(await homeRoot.evaluate((it) => it.className)).toContain("light");
});
