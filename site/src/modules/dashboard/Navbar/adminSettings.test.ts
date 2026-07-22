import {
	type AdminSettingsPermissions,
	canViewAdminSettings,
	getAdminSettingsItems,
} from "./adminSettings";

const noPermissions: AdminSettingsPermissions = {
	canViewDeployment: false,
	canViewOrganizations: false,
	canViewAuditLog: false,
	canViewConnectionLog: false,
	canViewAIBridge: false,
	canViewAISettings: false,
	canViewHealth: false,
};

describe("canViewAdminSettings", () => {
	it("does not surface the menu for organization access alone", () => {
		expect(
			canViewAdminSettings({ ...noPermissions, canViewOrganizations: true }),
		).toBe(false);
	});

	it("surfaces the menu for a genuine admin permission", () => {
		expect(
			canViewAdminSettings({ ...noPermissions, canViewDeployment: true }),
		).toBe(true);
	});

	it("returns false without any permission", () => {
		expect(canViewAdminSettings(noPermissions)).toBe(false);
	});
});

describe("getAdminSettingsItems", () => {
	it("always includes Organizations", () => {
		const items = getAdminSettingsItems(noPermissions);
		expect(items).toContainEqual({
			label: "Organizations",
			to: "/organizations",
		});
	});
});
