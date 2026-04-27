import { expect, type Page, test } from "@playwright/test";
import { users } from "../../constants";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(({ page }) => {
	beforeCoderTest(page);
});

const NON_LIGHT_THEME_CLASSES = [
	"dark",
	"dark-protan-deuter",
	"light-protan-deuter",
	"dark-tritan",
	"light-tritan",
] as const;

const expectPlainLightTheme = async (page: Page) => {
	const classes = await page
		.locator("html")
		.evaluate((it) => Array.from(it.classList));

	expect(classes).toContain("light");
	for (const themeClassName of NON_LIGHT_THEME_CLASSES) {
		expect(classes).not.toContain(themeClassName);
	}
};

test("adjust user theme preference", async ({ page }) => {
	await login(page, users.member);

	await page.goto("/settings/appearance", { waitUntil: "domcontentloaded" });

	await page.getByText("Light", { exact: true }).click();
	await expect(page.getByLabel("Light")).toBeChecked();

	// Make sure the page is actually updated to use the plain light theme.
	// ThemeProvider applies both the concrete theme class (e.g. `light`) and
	// the base mode class (`light`). A colorblind variant like `light-tritan`
	// would add `light-tritan` alongside `light`, so we assert on theme
	// class-list tokens to distinguish the plain `light` theme from any
	// colorblind variant without rejecting unrelated root classes.
	await expectPlainLightTheme(page);

	await page.goto("/", { waitUntil: "domcontentloaded" });

	// Make sure the page is still using the light theme after reloading and
	// navigating away from the settings page.
	await expectPlainLightTheme(page);
});
