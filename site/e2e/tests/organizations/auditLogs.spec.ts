import { expect, test } from "@playwright/test";
import {
	createOrganization,
	createOrganizationMember,
	setupApiCalls,
} from "../../api";
import { defaultPassword, users } from "../../constants";
import { login, randomName, requiresLicense } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.describe.configure({ mode: "parallel" });

const orgName = randomName();

const orgAuditor = {
	username: `org-auditor-${orgName}`,
	password: defaultPassword,
	email: `org-auditor-${orgName}@coder.com`,
};

test.beforeEach(({ page }) => {
	beforeCoderTest(page);
});

test.describe("organization scoped audit logs", () => {
	requiresLicense();

	test.beforeAll(async ({ browser }) => {
		const context = await browser.newContext();
		const page = await context.newPage();

		await login(page);
		await setupApiCalls(page);

		const org = await createOrganization(orgName);
		await createOrganizationMember({
			...orgAuditor,
			orgRoles: {
				[org.id]: ["organization-auditor"],
			},
		});

		await context.close();
	});

	test("organization auditors cannot see logins", async ({ page }) => {
		// Go to the audit history
		await login(page, orgAuditor);
		await page.goto("/audit");
		const username = orgAuditor.username;

		const loginMessage = `${username} logged in`;
		// Make sure those things we did all actually show up
		await expect(page.getByText(loginMessage).first()).not.toBeVisible();
	});

	test("creating organization is logged", async ({ page }) => {
		await login(page, orgAuditor);

		// Go to the audit history
		await page.goto("/audit", { waitUntil: "domcontentloaded" });

		const auditLogText = `${users.owner.username} created organization ${orgName}`;
		const org = page.locator(".MuiTableRow-root", {
			hasText: auditLogText,
		});
		await org.scrollIntoViewIfNeeded();
		await expect(org).toBeVisible();

		await org.getByLabel("open-dropdown").click();
		await expect(org.getByText(`icon: "/emojis/1f957.png"`)).toBeVisible();
	});

	test("assigning an organization role is logged", async ({ page }) => {
		await login(page, orgAuditor);

		// Go to the audit history
		await page.goto("/audit", { waitUntil: "domcontentloaded" });

		const auditLogText = `${users.owner.username} updated organization member ${orgAuditor.username}`;
		const member = page.locator(".MuiTableRow-root", {
			hasText: auditLogText,
		});
		await member.scrollIntoViewIfNeeded();
		await expect(member).toBeVisible();

		await member.getByLabel("open-dropdown").click();
		await expect(
			member.getByText(`roles: ["organization-auditor"]`),
		).toBeVisible();
	});
});
