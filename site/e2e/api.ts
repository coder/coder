import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { formatDuration, intervalToDuration } from "date-fns";
import { type DeploymentConfig, API } from "api/api";
import type { SerpentOption } from "api/typesGenerated";
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

export async function verifyConfigFlagBoolean(
  page: Page,
  config: DeploymentConfig,
  flag: string,
) {
  const opt = findConfigOption(config, flag);
  const type = opt.value ? "option-enabled" : "option-disabled";
  const value = opt.value ? "Enabled" : "Disabled";

  const configOption = page.locator(
    `div.options-table .option-${flag} .${type}`,
  );
  await expect(configOption).toHaveText(value);
}

export async function verifyConfigFlagNumber(
  page: Page,
  config: DeploymentConfig,
  flag: string,
) {
  const opt = findConfigOption(config, flag);
  const configOption = page.locator(
    `div.options-table .option-${flag} .option-value-number`,
  );
  await expect(configOption).toHaveText(String(opt.value));
}

export async function verifyConfigFlagString(
  page: Page,
  config: DeploymentConfig,
  flag: string,
) {
  const opt = findConfigOption(config, flag);

  const configOption = page.locator(
    `div.options-table .option-${flag} .option-value-string`,
  );
  await expect(configOption).toHaveText(opt.value);
}

export async function verifyConfigFlagEmpty(page: Page, flag: string) {
  const configOption = page.locator(
    `div.options-table .option-${flag} .option-value-empty`,
  );
  await expect(configOption).toHaveText("Not set");
}

export async function verifyConfigFlagArray(
  page: Page,
  config: DeploymentConfig,
  flag: string,
) {
  const opt = findConfigOption(config, flag);
  const configOption = page.locator(
    `div.options-table .option-${flag} .option-array`,
  );

  // Verify array of options with simple dots
  for (const item of opt.value) {
    await expect(configOption.locator("li", { hasText: item })).toBeVisible();
  }
}

export async function verifyConfigFlagEntries(
  page: Page,
  config: DeploymentConfig,
  flag: string,
) {
  const opt = findConfigOption(config, flag);
  const configOption = page.locator(
    `div.options-table .option-${flag} .option-array`,
  );

  // Verify array of options with green marks.
  Object.entries(opt.value)
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(async ([item]) => {
      await expect(
        configOption.locator(`.option-array-item-${item}.option-enabled`, {
          hasText: item,
        }),
      ).toBeVisible();
    });
}

export async function verifyConfigFlagDuration(
  page: Page,
  config: DeploymentConfig,
  flag: string,
) {
  const opt = findConfigOption(config, flag);
  const configOption = page.locator(
    `div.options-table .option-${flag} .option-value-string`,
  );
  await expect(configOption).toHaveText(
    formatDuration(
      // intervalToDuration takes ms, so convert nanoseconds to ms
      intervalToDuration({
        start: 0,
        end: (opt.value as number) / 1e6,
      }),
    ),
  );
}

export function findConfigOption(
  config: DeploymentConfig,
  flag: string,
): SerpentOption {
  const opt = config.options.find((option) => option.flag === flag);
  if (opt === undefined) {
    // must be undefined as `false` is expected
    throw new Error(`Option with env ${flag} has undefined value.`);
  }
  return opt;
}
