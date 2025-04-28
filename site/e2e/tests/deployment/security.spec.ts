import type { Page } from "@playwright/test";
import { expect, test } from "@playwright/test";
import { API, type DeploymentConfig } from "api/api";
import {
	findConfigOption,
	setupApiCalls,
	verifyConfigFlagBoolean,
	verifyConfigFlagNumber,
	verifyConfigFlagString,
} from "../../api";
import { login } from "../../helpers";
import { beforeCoderTest } from "../../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
	await setupApiCalls(page);
});

test("enabled security settings", async ({ page }) => {
	const config = await API.getDeploymentConfig();

	await page.goto("/deployment/security", { waitUntil: "domcontentloaded" });

	await verifyConfigFlagString(page, config, "ssh-keygen-algorithm");
	await verifyConfigFlagBoolean(page, config, "secure-auth-cookie");
	await verifyConfigFlagBoolean(page, config, "disable-owner-workspace-access");

	await verifyConfigFlagBoolean(page, config, "tls-redirect-http-to-https");
	await verifyStrictTransportSecurity(page, config);
	await verifyConfigFlagString(page, config, "tls-address");
	await verifyConfigFlagBoolean(page, config, "tls-allow-insecure-ciphers");
	await verifyConfigFlagString(page, config, "tls-client-auth");
	await verifyConfigFlagBoolean(page, config, "tls-enable");
	await verifyConfigFlagString(page, config, "tls-min-version");
});

async function verifyStrictTransportSecurity(
	page: Page,
	config: DeploymentConfig,
) {
	const flag = "strict-transport-security";
	const opt = findConfigOption(config, flag);
	if (opt.value !== 0) {
		await verifyConfigFlagNumber(page, config, flag);
		return;
	}

	const configOption = page.locator(
		`div.options-table .option-${flag} .option-value-string`,
	);
	await expect(configOption).toHaveText("Disabled");
}
