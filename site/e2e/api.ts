import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import * as API from "api/api";
import { coderPort } from "./constants";
import { findSessionToken, randomName } from "./helpers";

let currentOrgId: string;

export const setupApiCalls = async (page: Page) => {
  try {
    const token = await findSessionToken(page);
    API.setSessionToken(token);
  } catch {
    // If this fails, we have an unauthenticated client.
  }
  API.setHost(`http://127.0.0.1:${coderPort}`);
};

export const getCurrentOrgId = async (): Promise<string> => {
  if (currentOrgId) {
    return currentOrgId;
  }
  const currentUser = await API.getAuthenticatedUser();
  currentOrgId = currentUser.organization_ids[0];
  return currentOrgId;
};

export const createUser = async (orgId: string) => {
  const name = randomName();
  const user = await API.createUser({
    email: `${name}@coder.com`,
    username: name,
    password: "s3cure&password!",
    login_type: "password",
    disable_login: false,
    organization_id: orgId,
  });
  return user;
};

export const createGroup = async (orgId: string) => {
  const name = randomName();
  const group = await API.createGroup(orgId, {
    name,
    display_name: `Display ${name}`,
    avatar_url: "/emojis/1f60d.png",
    quota_allowance: 0,
  });
  return group;
};

export async function verifyConfigFlag(
  page: Page,
  config: API.DeploymentConfig,
  flag: string,
) {
  const opt = config.options.find((option) => option.flag === flag);
  if (opt === undefined) {
    // must be undefined as `false` is expected
    throw new Error(`Option with env ${flag} has undefined value.`);
  }

  // Map option type to test class name.
  let type: string;
  let value = opt.value;

  if (typeof value === "boolean") {
    // Boolean options map to string (Enabled/Disabled).
    type = value ? "option-enabled" : "option-disabled";
    value = value ? "Enabled" : "Disabled";
  } else if (typeof value === "number") {
    type = "option-value-number";
    value = String(value);
  } else if (!value || value.length === 0) {
    type = "option-value-empty";
  } else if (typeof value === "string") {
    type = "option-value-string";
  } else if (typeof value === "object") {
    type = "option-array";
  } else {
    type = "option-value-json";
  }

  // Special cases
  if (opt.flag === "strict-transport-security" && opt.value === 0) {
    type = "option-value-string";
    value = "Disabled"; // Display "Disabled" instead of zero seconds.
  }

  const configOption = page.locator(
    `div.options-table .option-${flag} .${type}`,
  );

  // Verify array of options with green marks.
  if (typeof value === "object" && !Array.isArray(value)) {
    Object.entries(value)
      .sort((a, b) => a[0].localeCompare(b[0]))
      .map(async ([item]) => {
        await expect(
          configOption.locator(`.option-array-item-${item}.option-enabled`, {
            hasText: item,
          }),
        ).toBeVisible();
      });
    return;
  }
  // Verify array of options with simmple dots
  if (Array.isArray(value)) {
    for (const item of value) {
      await expect(configOption.locator("li", { hasText: item })).toBeVisible();
    }
    return;
  }
  await expect(configOption).toHaveText(String(value));
}
