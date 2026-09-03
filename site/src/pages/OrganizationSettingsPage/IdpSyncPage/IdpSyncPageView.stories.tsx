import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import {
	MockGroup,
	MockGroup2,
	MockGroup3,
	MockGroupSyncSettings,
	MockGroupSyncSettings2,
	MockLegacyMappingGroupSyncSettings,
	MockMultipleOverflowGroupSyncSettings,
	MockOrganization,
	MockRoleSyncSettings,
} from "#/testHelpers/entities";
import IdpSyncPageView from "./IdpSyncPageView";

const groupsMap = new Map<string, string>();
for (const group of [MockGroup, MockGroup2, MockGroup3]) {
	groupsMap.set(group.id, group.display_name || group.name);
}

const hoverUnknownClaimWarning = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const warnings = canvas.getAllByRole("button", {
		name: "Unknown claim value",
	});
	const warning = warnings[0];
	if (!warning) {
		throw new Error("Expected an unknown claim warning");
	}
	await userEvent.hover(warning);
	await screen.findByRole("tooltip", {
		name: /has not be seen in the specified claim field/i,
	});
};

const meta: Meta<typeof IdpSyncPageView> = {
	title: "pages/IdpSyncPage",
	component: IdpSyncPageView,
	args: {
		groupSyncSettings: MockGroupSyncSettings,
		roleSyncSettings: MockRoleSyncSettings,
		groupClaimFieldValues: Object.keys(MockGroupSyncSettings.mapping),
		roleClaimFieldValues: Object.keys(MockRoleSyncSettings.mapping),
		groups: [MockGroup, MockGroup2],
		groupsMap,
		organization: MockOrganization,
		error: undefined,
	},
};

export default meta;
type Story = StoryObj<typeof IdpSyncPageView>;

export const Empty: Story = {
	args: {
		groupSyncSettings: {
			field: "",
			mapping: {},
			regex_filter: "",
			auto_create_missing_groups: false,
		},
		roleSyncSettings: {
			field: "",
			mapping: {},
		},
		groups: [],
		groupsMap: undefined,
		organization: MockOrganization,
		error: undefined,
	},
};

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Group sync", level: 2 }),
		).toBeVisible();
		await expect(
			canvas.getByRole("heading", { name: "Role sync", level: 2 }),
		).toBeVisible();
		await expect(
			canvas.getAllByRole("heading", { name: "Sync field", level: 3 }),
		).toHaveLength(2);
		await expect(
			canvas.getByRole("heading", { name: "Group mapping", level: 3 }),
		).toBeVisible();
		await expect(
			canvas.getByRole("heading", { name: "Role mapping", level: 3 }),
		).toBeVisible();
		await expect(
			canvas.getByRole("switch", { name: /auto create missing groups/i }),
		).toBeVisible();
		await expect(
			canvas.getAllByRole("button", { name: /export policy/i }),
		).toHaveLength(2);
		await expect(
			canvas.queryByRole("heading", { name: "Legacy group sync" }),
		).not.toBeInTheDocument();
		await expect(canvas.queryByRole("tab")).not.toBeInTheDocument();
	},
};

export const HasError: Story = {
	args: {
		error: "This is a test error",
	},
};

export const MissingGroups: Story = {
	args: {
		groupSyncSettings: MockGroupSyncSettings2,
	},
};

export const MultipleOverflowGroups: Story = {
	args: {
		groupSyncSettings: MockMultipleOverflowGroupSyncSettings,
	},
};

export const WithLegacyMapping: Story = {
	args: {
		groupSyncSettings: MockLegacyMappingGroupSyncSettings,
		groupClaimFieldValues: Object.keys(
			MockLegacyMappingGroupSyncSettings.legacy_group_name_mapping,
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Legacy group sync", level: 3 }),
		).toBeVisible();
	},
};

export const MissingGroupClaims: Story = {
	args: {
		groupClaimFieldValues: [],
	},
	play: async ({ canvasElement }) => {
		await hoverUnknownClaimWarning(canvasElement);
	},
};

export const MissingRoleClaims: Story = {
	args: {
		roleClaimFieldValues: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const roleRow = canvas.getByRole("row", { name: /idp-role-1/ });
		const warning = within(roleRow).getByRole("button", {
			name: "Unknown claim value",
		});
		await userEvent.hover(warning);
		await screen.findByRole("tooltip", {
			name: /has not be seen in the specified claim field/i,
		});
	},
};
