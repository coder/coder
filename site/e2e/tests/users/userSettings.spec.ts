import { expect, type Page, test } from "@playwright/test";
import { CONCRETE_THEMES } from "#/theme";
import { users } from "../../constants";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(({ page }) => {
	beforeCoderTest(page);
});

const rootClassNames = async (page: Page) => {
	return page.locator("html").evaluate((it) => Array.from(it.classList));
};

const expectLightThemeClasses = async (page: Page) => {
	await expect(async () => {
		const classes = await rootClassNames(page);
		const className = "light";

		// Assert the light theme without rejecting unrelated root classes.
		expect(classes).toContain(className);
		for (const themeClassName of CONCRETE_THEMES.filter(
			(it) => it !== className,
		)) {
			expect(classes).not.toContain(themeClassName);
		}
	}).toPass({ timeout: 10_000 });
};

test("adjust user theme preference", async ({ page }) => {
	await login(page, users.member);

	await page.goto("/settings/appearance", { waitUntil: "domcontentloaded" });

	await page.getByRole("combobox", { name: /theme mode/i }).click();
	await page.getByRole("option", { name: /single theme/i }).click();

	const singleThemeGroup = page.getByRole("group", { name: "Theme" });
	await expect(singleThemeGroup).toBeVisible();
	await singleThemeGroup.getByText("Light default", { exact: true }).click();

	await expectLightThemeClasses(page);

	await page.goto("/", { waitUntil: "domcontentloaded" });

	// Make sure the page is still using the light theme after reloading and
	// navigating away from the settings page.
	await expectLightThemeClasses(page);
});
