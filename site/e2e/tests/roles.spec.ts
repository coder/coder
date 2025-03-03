import { type Page, expect, test } from "@playwright/test";
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
	// Organizations and Audit Logs both require a license to be visible
	const visibleSettings = license
		? settings
		: settings.filter((it) => it !== "Organizations" && it !== "Audit Logs");
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

		await hasAccessToAdminSettings(page, ["Deployment", "Organizations"]);
	});

	test("user admin can see admin settings", async ({ page }) => {
		await login(page, users.userAdmin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Deployment", "Organizations"]);
	});

	test("auditor can see admin settings", async ({ page }) => {
		await login(page, users.auditor);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, [
			"Deployment",
			"Organizations",
			"Audit Logs",
		]);
	});

	test("admin can see admin settings", async ({ page }) => {
		await login(page, users.admin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, [
			"Deployment",
			"Organizations",
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

	test("org template admin can see admin settings", async ({ page }) => {
		const org = await createOrganization();
		const orgTemplateAdmin = await createOrganizationMember({
			[org.id]: ["organization-template-admin"],
		});

		await login(page, orgTemplateAdmin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Organizations"]);
	});

	test("org user admin can see admin settings", async ({ page }) => {
		const org = await createOrganization();
		const orgUserAdmin = await createOrganizationMember({
			[org.id]: ["organization-user-admin"],
		});

		await login(page, orgUserAdmin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Deployment", "Organizations"]);
	});

	test("org auditor can see admin settings", async ({ page }) => {
		const org = await createOrganization();
		const orgAuditor = await createOrganizationMember({
			[org.id]: ["organization-auditor"],
		});

		await login(page, orgAuditor);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await hasAccessToAdminSettings(page, ["Organizations", "Audit Logs"]);
	});

	test("org admin can see admin settings", async ({ page }) => {
		const org = await createOrganization();
		const orgAdmin = await createOrganizationMember({
			[org.id]: ["organization-admin"],
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
