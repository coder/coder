import { test } from "@playwright/test";
import { API } from "api/api";
import {
	setupApiCalls,
	verifyConfigFlagArray,
	verifyConfigFlagBoolean,
	verifyConfigFlagEntries,
	verifyConfigFlagString,
} from "../../api";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test("login with OIDC", async ({ page }) => {
	const config = await API.getDeploymentConfig();

	await page.goto("/deployment/userauth", { waitUntil: "domcontentloaded" });

	await verifyConfigFlagBoolean(page, config, "oidc-group-auto-create");
	await verifyConfigFlagBoolean(page, config, "oidc-allow-signups");
	await verifyConfigFlagEntries(page, config, "oidc-auth-url-params");
	await verifyConfigFlagString(page, config, "oidc-client-id");
	await verifyConfigFlagArray(page, config, "oidc-email-domain");
	await verifyConfigFlagString(page, config, "oidc-email-field");
	await verifyConfigFlagEntries(page, config, "oidc-group-mapping");
	await verifyConfigFlagBoolean(page, config, "oidc-ignore-email-verified");
	await verifyConfigFlagBoolean(page, config, "oidc-ignore-userinfo");
	await verifyConfigFlagString(page, config, "oidc-issuer-url");
	await verifyConfigFlagString(page, config, "oidc-group-regex-filter");
	await verifyConfigFlagArray(page, config, "oidc-scopes");
	await verifyConfigFlagEntries(page, config, "oidc-user-role-mapping");
	await verifyConfigFlagString(page, config, "oidc-username-field");
	await verifyConfigFlagString(page, config, "oidc-sign-in-text");
	await verifyConfigFlagString(page, config, "oidc-icon-url");
});
