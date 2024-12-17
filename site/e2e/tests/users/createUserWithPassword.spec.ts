import { test } from "@playwright/test";
import { createUser } from "../../helpers";
import { beforeCoderTest } from "../../hooks";
import { login } from "../../helpers";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("create user with password", async ({ page }) => {
	await createUser(page);
});

test("create user without full name", async ({ page }) => {
	await createUser(page, { name: "" });
});
