import { expect, test } from "@playwright/test";
import {
	createGroup,
	createUser,
	getCurrentOrgId,
	setupApiCalls,
} from "../../api";
import { defaultOrganizationName } from "../../constants";
import { requiresLicense } from "../../helpers";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test("add members", async ({ page, baseURL }) => {
	requiresLicense();

	const orgName = defaultOrganizationName;
	const orgId = await getCurrentOrgId();
	const group = await createGroup(orgId);
	const numberOfMembers = 3;
	const users = await Promise.all(
		Array.from({ length: numberOfMembers }, () => createUser(orgId)),
	);

	await page.goto(`${baseURL}/organizations/${orgName}/groups/${group.name}`, {
		waitUntil: "domcontentloaded",
	});
	await expect(page).toHaveTitle(`${group.display_name} - Coder`);

	for (const user of users) {
		await page.getByPlaceholder("User email or username").fill(user.username);
		await page.getByRole("option", { name: user.email }).click();
		await page.getByRole("button", { name: "Add user" }).click();
		await expect(page.getByRole("row", { name: user.username })).toBeVisible();
	}
});
