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

	// Switch the theme-mode dropdown to "Single theme" so we can pick a
	// specific variant, independent of the test runner's OS color scheme.
	await page.getByRole("combobox", { name: /theme mode/i }).click();
	await page.getByRole("option", { name: /single theme/i }).click();

	// Pick the default light theme by clicking its tile. The tile label
	// matches the title string used by SingleModeSection.
	await page.getByText("Light default", { exact: true }).click();

	// Make sure the page is actually updated to use the light theme.
	// Match as a whole word so a colorblind variant like "light-tritan"
	// never satisfies this assertion.
	const [root] = await page.$$("html");
	expect(await root.evaluate((it) => it.className)).toMatch(/\blight\b/);

	await page.goto("/", { waitUntil: "domcontentloaded" });

	// Make sure the page is still using the light theme after reloading and
	// navigating away from the settings page.
	const [homeRoot] = await page.$$("html");
	expect(await homeRoot.evaluate((it) => it.className)).toMatch(/\blight\b/);
});
