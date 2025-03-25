import { expect, test } from "@playwright/test";
import { API } from "api/api";
import {
	createGroup,
	createUser,
	getCurrentOrgId,
	setupApiCalls,
} from "../../api";
import { defaultOrganizationName, users } from "../../constants";
import { login, requiresLicense } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page, users.userAdmin);
	await setupApiCalls(page);
});

test("remove member", async ({ page, baseURL }) => {
	requiresLicense();

	const orgName = defaultOrganizationName;
	const orgId = await getCurrentOrgId();
	const [group, member] = await Promise.all([
		createGroup(orgId),
		createUser(orgId),
	]);
	await API.addMember(group.id, member.id);

	await page.goto(`${baseURL}/organizations/${orgName}/groups/${group.name}`, {
		waitUntil: "domcontentloaded",
	});
	await expect(page).toHaveTitle(`${group.display_name} - Coder`);

	const userRow = page.getByRole("row", { name: member.username });
	await userRow.getByRole("button", { name: "More options" }).click();

	const menu = page.locator("#more-options");
	await menu.getByText("Remove").click({ timeout: 1_000 });

	await expect(page.getByText("Member removed successfully.")).toBeVisible();
});
