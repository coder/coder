import { test } from "@playwright/test";
import { getDeploymentConfig } from "api/api";
import { setupApiCalls, verifyConfigFlag } from "../../api";

test("login with OIDC", async ({ page }) => {
  await setupApiCalls(page);
  const config = await getDeploymentConfig();

  await page.goto("/deployment/userauth", { waitUntil: "domcontentloaded" });

  const flags = [
    "oidc-group-auto-create",
    "oidc-allow-signups",
    "oidc-auth-url-params",
    "oidc-client-id",
    "oidc-email-domain",
    "oidc-email-field",
    "oidc-group-mapping",
    "oidc-ignore-email-verified",
    "oidc-ignore-userinfo",
    "oidc-issuer-url",
    "oidc-group-regex-filter",
    "oidc-scopes",
    "oidc-user-role-mapping",
    "oidc-username-field",
    "oidc-sign-in-text",
    "oidc-icon-url",
  ];

  for (const flag of flags) {
    await verifyConfigFlag(page, config, flag);
  }
});
