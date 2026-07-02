import { expect, test } from "@playwright/test";
import { users } from "../constants";
import { expectUrl } from "../expectUrl";
import { beforeCoderTest } from "../hooks";

test.beforeEach(({ page }) => {
	beforeCoderTest(page);
});

// The post-login redirect to `/workspaces` must not depend on
// `POST /api/v2/authcheck`. Inject the CSRF middleware's 400 response on
// every authcheck request and assert the redirect still happens.
test("login redirect survives a CSRF failure on /api/v2/authcheck", async ({
	page,
}) => {
	await page.context().clearCookies();

	let authcheckHits = 0;
	await page.route("**/api/v2/authcheck", async (route) => {
		authcheckHits++;
		await route.fulfill({
			status: 400,
			contentType: "text/plain",
			body: "Something is wrong with your CSRF token. Please refresh the page. If this error persists, try clearing your cookies.\n",
		});
	});

	await page.goto("/login");
	await page.getByLabel("Email").fill(users.owner.email);
	await page.getByLabel("Password").fill(users.owner.password);
	await page.getByRole("button", { name: "Sign In" }).click();

	await expectUrl(page).toHavePathName("/workspaces", { timeout: 15000 });
	// Without this, the test could pass vacuously if the SPA stopped calling
	// authcheck on the login path entirely.
	expect(authcheckHits).toBeGreaterThan(0);
});
