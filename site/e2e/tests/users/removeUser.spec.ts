import { expect, test } from "@playwright/test";
import { createUser, getCurrentOrgId, setupApiCalls } from "../../api";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test("remove user", async ({ page, baseURL }) => {
	const orgId = await getCurrentOrgId();
	const user = await createUser(orgId);

	await page.goto(`${baseURL}/users`, { waitUntil: "domcontentloaded" });
	await expect(page).toHaveTitle("Users - Coder");

	const userRow = page.getByRole("row", { name: user.email });
	await userRow.getByRole("button", { name: "More options" }).click();
	const menu = page.locator("#more-options");
	await menu.getByText("Delete").click();

	const dialog = page.getByTestId("dialog");
	await dialog.getByLabel("Name of the user to delete").fill(user.username);
	await dialog.getByRole("button", { name: "Delete" }).click();

	await expect(page.getByText("Successfully deleted the user.")).toBeVisible();
});
