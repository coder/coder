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

	// Precondition: the theme mode must start on "Single theme" so that picking
	// a single theme below takes effect immediately. A fresh member defaults to
	// single mode; assert it so the test fails loudly if that default changes.
	await expect(
		page.getByRole("combobox", { name: /theme mode/i }),
	).toContainText("Single theme");

	const singleThemeGroup = page.getByRole("group", { name: "Theme" });
	await expect(singleThemeGroup).toBeVisible();
	await singleThemeGroup.getByText("Light default", { exact: true }).click();

	await expectLightThemeClasses(page);

	// The theme is saved optimistically, so the DOM turns light before the
	// preference is persisted. The form shows a spinner while the save is in
	// flight; wait for it to clear so the save has completed (and was not
	// canceled by navigation) before the hard reload. Asserting the optimistic
	// class first guarantees the spinner is already showing if a save started,
	// and a repeat run that is already light simply never shows it.
	await expect(
		page.getByRole("status", { name: "Saving theme preference" }),
	).toBeHidden();

	await page.goto("/", { waitUntil: "domcontentloaded" });

	// Make sure the page is still using the light theme after reloading and
	// navigating away from the settings page.
	await expectLightThemeClasses(page);
});
