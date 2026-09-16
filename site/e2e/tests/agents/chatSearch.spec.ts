import { expect, test } from "@playwright/test";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("searches chats with backend full-text search", async ({ page }) => {
	await page.goto("/agents", { waitUntil: "domcontentloaded" });

	await page.getByRole("button", { name: "Search chats" }).first().click();
	const searchInput = page.getByRole("combobox", { name: "Search chats" });
	await expect(searchInput).toBeVisible();

	const searchResponse = page.waitForResponse((response) => {
		const url = new URL(response.url());
		return (
			url.pathname === "/api/v2/chats" &&
			url.searchParams.get("q") === 'search:"full-text-smoke"'
		);
	});
	await searchInput.fill("full-text-smoke");

	await expect((await searchResponse).status()).toBe(200);
	await expect(
		page.getByText("No matching chats", { exact: false }),
	).toBeVisible();
	await expect(page.getByRole("alert")).not.toBeVisible();
});
