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

// Assert the light theme without rejecting unrelated root classes.
const expectLightThemeClasses = (classes: string[]) => {
	const className = "light";
	expect(classes).toContain(className);
	for (const themeClassName of CONCRETE_THEMES.filter(
		(it) => it !== className,
	)) {
		expect(classes).not.toContain(themeClassName);
	}
};

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

	expectLightThemeClasses(await rootClassNames(page));

	await page.goto("/", { waitUntil: "domcontentloaded" });

	// Make sure the page is still using the light theme after reloading and
	// navigating away from the settings page.
	expectLightThemeClasses(await rootClassNames(page));
});
