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
	email: "e2e-tests@coder.com",
	companyName: "Coder E2E Test Suite",
	jobTitle: "QA Automation",
	phoneNumber: "+14155550100",
	developers: "1 - 50",
	country: "United States",
};

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
			await page.getByRole("option", { name: testIdentity.developers }).click();

			await page.getByLabel("Country").click();
			await page.getByRole("option", { name: testIdentity.country }).click();

			await page.getByLabel(/I understand that Premium features/).click();

			// This is a real submission to the live Coder licensor.
			await submitButton.click();

			await expectUrl(page).toHavePathName("/deployment/licenses", {
				timeout: 30_000,
			});
			expect(new URL(page.url()).searchParams.get("success")).toBe("true");

			const firstLicense = page.locator(".licenses > .license-card", {
				hasText: "#1",
			});
			await expect(firstLicense).toBeVisible();
			await expect(firstLicense.locator(".license-type")).toHaveText("Trial");

			const entitlements = await API.getEntitlements();
			expect(entitlements.has_license).toBe(true);
			expect(entitlements.trial).toBe(true);
		});

		test("removing the license resets the premium page", async ({ page }) => {
			await page.goto("/deployment/licenses", {
				waitUntil: "domcontentloaded",
			});

			const firstLicense = page.locator(".licenses > .license-card", {
				hasText: "#1",
			});
			await expect(firstLicense).toBeVisible();

			await firstLicense
				.getByRole("button", { name: "Show license actions" })
				.click();
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
