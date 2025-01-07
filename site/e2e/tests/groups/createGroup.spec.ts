import { expect, test } from "@playwright/test";
import { randomName, requiresLicense } from "../../helpers";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("create group", async ({ page, baseURL }) => {
	requiresLicense();

	await page.goto(`${baseURL}/groups`, { waitUntil: "domcontentloaded" });
	await expect(page).toHaveTitle("Groups - Coder");

	await page.getByText("Create group").click();
	await expect(page).toHaveTitle("Create Group - Coder");

	const name = randomName();
	const groupValues = {
		name: name,
		displayName: `Display Name for ${name}`,
		avatarURL: "/emojis/1f60d.png",
	};

	await page.getByLabel("Name", { exact: true }).fill(groupValues.name);
	await page.getByLabel("Display Name").fill(groupValues.displayName);
	await page.getByLabel("Avatar URL").fill(groupValues.avatarURL);
	await page.getByRole("button", { name: /save/i }).click();

	await expect(page).toHaveTitle(`${groupValues.displayName} - Coder`);
	await expect(page.getByText(groupValues.displayName)).toBeVisible();
	await expect(page.getByText("No members yet")).toBeVisible();
});
