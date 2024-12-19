import { expect, test } from "@playwright/test";
import { createGroup, getCurrentOrgId, setupApiCalls } from "../../api";
import { requiresLicense } from "../../helpers";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test("remove group", async ({ page, baseURL }) => {
	requiresLicense();

	const orgId = await getCurrentOrgId();
	const group = await createGroup(orgId);

	await page.goto(`${baseURL}/groups/${group.name}`, {
		waitUntil: "domcontentloaded",
	});
	await expect(page).toHaveTitle(`${group.display_name} - Coder`);

	await page.getByRole("button", { name: "Delete" }).click();
	const dialog = page.getByTestId("dialog");
	await dialog.getByLabel("Name of the group to delete").fill(group.name);
	await dialog.getByRole("button", { name: "Delete" }).click();
	await expect(page.getByText("Group deleted successfully.")).toBeVisible();

	await expect(page).toHaveTitle("Groups - Coder");
});
