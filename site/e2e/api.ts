import type { Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { API, type DeploymentConfig } from "api/api";
import type { SerpentOption } from "api/typesGenerated";
import { formatDuration, intervalToDuration } from "date-fns";
import { coderPort } from "./constants";
import { findSessionToken, randomName } from "./helpers";

let currentOrgId: string;

export const setupApiCalls = async (page: Page) => {
	API.setHost(`http://127.0.0.1:${coderPort}`);
	const token = await findSessionToken(page);
	API.setSessionToken(token);
};

export const getCurrentOrgId = async (): Promise<string> => {
	if (currentOrgId) {
		return currentOrgId;
	}
	const currentUser = await API.getAuthenticatedUser();
	currentOrgId = currentUser.organization_ids[0];
	return currentOrgId;
};

export const createUser = async (...orgIds: string[]) => {
	const name = randomName();
	const user = await API.createUser({
		email: `${name}@coder.com`,
		username: name,
		name: name,
		password: "s3cure&password!",
		login_type: "password",
		organization_ids: orgIds,
		user_status: null,
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

export const createOrganization = async () => {
	const name = randomName();
	const org = await API.createOrganization({
		name,
		display_name: `Org ${name}`,
		description: `Org description ${name}`,
		icon: "/emojis/1f957.png",
	});
	return org;
};

export const deleteOrganization = async (orgName: string) => {
	await API.deleteOrganization(orgName);
};

export const createOrganizationWithName = async (name: string) => {
	const org = await API.createOrganization({
		name,
		display_name: `${name}`,
		description: `Org description ${name}`,
		icon: "/emojis/1f957.png",
	});
	return org;
};

export const createOrganizationSyncSettings = async () => {
	const settings = await API.patchOrganizationIdpSyncSettings({
		field: "organization-field-test",
		mapping: {
			"idp-org-1": [
				"fbd2116a-8961-4954-87ae-e4575bd29ce0",
				"13de3eb4-9b4f-49e7-b0f8-0c3728a0d2e2",
			],
			"idp-org-2": ["6b39f0f1-6ad8-4981-b2fc-d52aef53ff1b"],
		},
		organization_assign_default: true,
	});
	return settings;
};

export const createGroupSyncSettings = async (orgId: string) => {
	const settings = await API.patchGroupIdpSyncSettings(
		{
			field: "group-field-test",
			mapping: {
				"idp-group-1": [
					"fbd2116a-8961-4954-87ae-e4575bd29ce0",
					"13de3eb4-9b4f-49e7-b0f8-0c3728a0d2e2",
				],
				"idp-group-2": ["6b39f0f1-6ad8-4981-b2fc-d52aef53ff1b"],
			},
			regex_filter: "@[a-zA-Z0-9]+",
			auto_create_missing_groups: true,
		},
		orgId,
	);
	return settings;
};

export const createRoleSyncSettings = async (orgId: string) => {
	const settings = await API.patchRoleIdpSyncSettings(
		{
			field: "role-field-test",
			mapping: {
				"idp-role-1": [
					"fbd2116a-8961-4954-87ae-e4575bd29ce0",
					"13de3eb4-9b4f-49e7-b0f8-0c3728a0d2e2",
				],
				"idp-role-2": ["6b39f0f1-6ad8-4981-b2fc-d52aef53ff1b"],
			},
		},
		orgId,
	);
	return settings;
};

export const createCustomRole = async (
	orgId: string,
	name: string,
	displayName: string,
) => {
	const role = await API.createOrganizationRole(orgId, {
		name,
		display_name: displayName,
		organization_id: orgId,
		site_permissions: [],
		organization_permissions: [
			{
				negate: false,
				resource_type: "organization_member",
				action: "create",
			},
			{
				negate: false,
				resource_type: "organization_member",
				action: "delete",
			},
			{
				negate: false,
				resource_type: "organization_member",
				action: "read",
			},
			{
				negate: false,
				resource_type: "organization_member",
				action: "update",
			},
		],
		user_permissions: [],
	});
	return role;
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
	// biome-ignore lint/suspicious/noExplicitAny: opt.value is any
	await expect(configOption).toHaveText(opt.value as any);
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
	// biome-ignore lint/suspicious/noExplicitAny: opt.value is any
	for (const item of opt.value as any) {
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
	// biome-ignore lint/suspicious/noExplicitAny: opt.value is any
	Object.entries(opt.value as any)
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
	//
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
