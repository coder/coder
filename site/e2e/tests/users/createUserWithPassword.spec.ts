import { test } from "@playwright/test";
import { createUser } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => await beforeCoderTest(page));

test("create user with password", async ({ page }) => {
	await createUser(page);
});

test("create user without full name", async ({ page }) => {
	await createUser(page, { name: "" });
});
