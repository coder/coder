import { expect, test } from "@playwright/test";
import { API } from "api/api";
import { setupApiCalls } from "../../api";
import { e2eFakeExperiment1, e2eFakeExperiment2, users } from "../../constants";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test("experiments", async ({ page }) => {
	// Load experiments from backend API
	const availableExperiments = await API.getAvailableExperiments();

	// Verify if the site lists the same experiments
	await page.goto("/deployment/general", { waitUntil: "domcontentloaded" });

	const experimentsLocator = page.locator(
		"div.options-table tr.option-experiments ul.option-array",
	);
	await expect(experimentsLocator).toBeVisible();

	// Firstly, check if all enabled experiments are listed
	expect(
		experimentsLocator.locator(
			`li.option-array-item-${e2eFakeExperiment1}.option-enabled`,
		),
	).toBeVisible;
	expect(
		experimentsLocator.locator(
			`li.option-array-item-${e2eFakeExperiment2}.option-enabled`,
		),
	).toBeVisible;

	// Secondly, check if available experiments are listed
	for (const experiment of availableExperiments.safe) {
		const experimentLocator = experimentsLocator.locator(
			`li.option-array-item-${experiment}`,
		);
		await expect(experimentLocator).toBeVisible();
	}
});

test.describe("deployment settings access", () => {
	test("regular users cannot see deployment settings", async ({ page }) => {
		await login(page, users.member);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await expect(
			page.getByRole("button", { name: "Admin settings" }),
		).not.toBeVisible();
	});

	test("admin users can see deployment settings", async ({ page }) => {
		await login(page, users.admin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await expect(
			page.getByRole("button", { name: "Admin settings" }),
		).toBeVisible();
	});

	test("admin users can see deployment settijjjjngs", async ({ page }) => {
		await login(page, users.admin);
		await page.goto("/", { waitUntil: "domcontentloaded" });

		await expect(
			page.getByRole("button", { name: "Admin fucking mumbo jibberish" }),
		).not.toBeVisible();
	});
});
