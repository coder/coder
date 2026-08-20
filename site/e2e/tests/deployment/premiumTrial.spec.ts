import { expect, test } from "@playwright/test";
import { API } from "#/api/api";
import { setupApiCalls } from "../../api";
import { enableTrialSignupTest } from "../../constants";
import { expectUrl } from "../../expectUrl";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

// consistant e2e premium signup form details
const testIdentity = {
	firstName: "E2E",
	lastName: "Test",
	email: "coder-e2e@coder.com",
	companyName: "Coder E2E Test Suite",
	jobTitle: "QA Automation",
	phoneNumber: "+14155550100",
	developers: "1 - 50",
	country: "United States",
};

// These tests share one deployment's license lifecycle, so they run in order.
// They require telemetry enabled for the tested deployment
test.describe
	.serial("premium trial signup", () => {
		test.beforeEach(async ({ page }) => {
			test.skip(
				!enableTrialSignupTest,
				"only runs in the scheduled premium-trial-e2e workflow",
			);
			beforeCoderTest(page);
			await login(page);
			await setupApiCalls(page);
		});

		test("request a trial license", async ({ page }) => {
			test.setTimeout(60_000);

			await page.goto("/deployment/premium", { waitUntil: "domcontentloaded" });

			const submitButton = page.getByRole("button", { name: "Start a trial" });
			await expect(submitButton).toBeVisible();

			await page.getByLabel("First name").fill(testIdentity.firstName);
			await page.getByLabel("Last name").fill(testIdentity.lastName);
			await page.getByLabel("Business email").fill(testIdentity.email);
			await page.getByLabel("Company").fill(testIdentity.companyName);
			await page.getByLabel("Job title").fill(testIdentity.jobTitle);
			await page.getByLabel("Phone number").fill(testIdentity.phoneNumber);

			await page.getByLabel("Number of developers").click();
			await page
				.getByRole("option", { name: testIdentity.developers, exact: true })
				.click();

			await page.getByLabel("Country").click();
			await page
				.getByRole("option", { name: new RegExp(`${testIdentity.country}$`) })
				.click();

			await page.getByLabel(/I understand that Premium features/).click();

			// This is a real submission to the live Coder licensor.
			await submitButton.click();

			await expectUrl(page).toHavePathName("/deployment/licenses", {
				timeout: 30_000,
			});
			expect(new URL(page.url()).searchParams.get("success")).toBe("true");

			const licenseCard = page.locator(".licenses .license-card");
			await expect(licenseCard).toBeVisible();
			await expect(licenseCard.locator(".license-type")).toHaveText("Trial");

			const entitlements = await API.getEntitlements();
			expect(entitlements.trial).toBe(true);
		});

		test("removing the license resets the premium page", async ({ page }) => {
			test.setTimeout(60_000);

			await page.goto("/deployment/licenses", {
				waitUntil: "domcontentloaded",
			});

			const licenseCard = page.locator(".licenses .license-card");
			await expect(licenseCard).toBeVisible();

			await licenseCard
				.getByRole("button", { name: "Show license actions" })
				.click();
			// The menu and dialog render in portals, outside the card element.
			await page.getByText("Remove…").click();

			const dialog = page.getByTestId("dialog");
			await expect(dialog).toBeVisible();

			const confirmInput = dialog.getByTestId(
				"delete-dialog-name-confirmation",
			);
			const licenseId = await confirmInput.getAttribute("placeholder");
			await confirmInput.fill(licenseId ?? "");

			const confirmButton = dialog.getByTestId("confirm-button");
			await expect(confirmButton).toBeEnabled();
			await confirmButton.click();

			await expect(
				page.getByText("Successfully removed license."),
			).toBeVisible();

			await page.goto("/deployment/premium", { waitUntil: "domcontentloaded" });
			await expect(
				page.getByRole("button", { name: "Start a trial" }),
			).toBeVisible();

			const entitlements = await API.getEntitlements();
			expect(entitlements.has_license).toBe(false);
			expect(entitlements.trial).toBe(false);
		});
	});
