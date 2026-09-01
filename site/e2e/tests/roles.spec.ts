import { expect, type Page, test } from "@playwright/test";
import {
	createOrganization,
	createOrganizationMember,
	setupApiCalls,
} from "../api";
import { license, users } from "../constants";
import { login, requiresLicense } from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
});

type AdminSetting = (typeof adminSettings)[number];

const adminSettings = [
	"Deployment",
	"Organizations",
	"Healthcheck",
	"Audit Logs",
] as const;

async function hasAccessToAdminSettings(page: Page, settings: AdminSetting[]) {
	// Audit Logs requires a license to be visible
	const visibleSettings = license
		? settings
		: settings.filter((it) => it !== "Audit Logs");
	const adminSettingsButton = page.getByRole("button", {
		name: "Admin settings",
	});
	if (visibleSettings.length < 1) {
		await expect(adminSettingsButton).not.toBeVisible();
		return;
	}

	await adminSettingsButton.click();

	for (const name of visibleSettings) {
		await expect(page.getByText(name, { exact: true })).toBeVisible();
	}

	const hiddenSettings = adminSettings.filter(
		(it) => !visibleSettings.includes(it),
	);
	for (const name of hiddenSettings) {
		await expect(page.getByText(name, { exact: true })).not.toBeVisible();
	}
}

test.describe("roles admin settings access", () => {
	test("member cannot see admin settings", async ({ page }) => {
		await login(page, users.member);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		// None, "Admin settings" button should not be visible
		await hasAccessToAdminSettings(page, []);
	});

	test("template admin can see admin settings", async ({ page }) => {
		await login(page, users.templateAdmin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Deployment"]);
	});

	test("user admin can see admin settings", async ({ page }) => {
		await login(page, users.userAdmin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Deployment"]);
	});

	test("auditor can see admin settings", async ({ page }) => {
		await login(page, users.auditor);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Deployment", "Audit Logs"]);
	});

	test("owner can see admin settings", async ({ page }) => {
		await login(page, users.owner);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, [
			"Deployment",
			"Healthcheck",
			"Audit Logs",
		]);
	});
});

test.describe("org-scoped roles admin settings access", () => {
	requiresLicense();

	test.beforeEach(async ({ page }) => {
		await login(page);
		await setupApiCalls(page);
	});

	test("member cannot see admin settings", async ({ page }) => {
		// The unlicensed member test above cannot catch all regressions here.
		// Many admin settings are locked behind a license and wouldn't be shown.
		const org = await createOrganization();
		const member = await createOrganizationMember({
			orgRoles: {
				[org.id]: [],
			},
		});

		await login(page, member);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		// None, "Admin settings" button should not be visible
		await hasAccessToAdminSettings(page, []);
	});

	test("org template admin can see admin settings", async ({ page }) => {
		const org = await createOrganization();
		const orgTemplateAdmin = await createOrganizationMember({
			orgRoles: {
				[org.id]: ["organization-template-admin"],
			},
		});

		await login(page, orgTemplateAdmin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Organizations"]);
	});

	test("org user admin can see admin settings", async ({ page }) => {
		const org = await createOrganization();
		const orgUserAdmin = await createOrganizationMember({
			orgRoles: {
				[org.id]: ["organization-user-admin"],
			},
		});

		await login(page, orgUserAdmin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Deployment", "Organizations"]);
	});

	test("org auditor can see admin settings", async ({ page }) => {
		const org = await createOrganization();
		const orgAuditor = await createOrganizationMember({
			orgRoles: {
				[org.id]: ["organization-auditor"],
			},
		});

		await login(page, orgAuditor);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Organizations", "Audit Logs"]);
	});

	test("org admin can see admin settings", async ({ page }) => {
		const org = await createOrganization();
		const orgAdmin = await createOrganizationMember({
			orgRoles: {
				[org.id]: ["organization-admin"],
			},
		});

		await login(page, orgAdmin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, [
			"Deployment",
			"Organizations",
			"Audit Logs",
		]);
	});
});
