import { MockNoPermissions, MockPermissions } from "#/testHelpers/entities";
import { canManageAnyOrganization, canViewAnyOrganization } from ".";

describe("organization permission helpers", () => {
	it("treats member read access as organization view access, not management", () => {
		const permissions = {
			...MockNoPermissions,
			viewAnyMembers: true,
		};

		expect(canViewAnyOrganization(permissions)).toBe(true);
		expect(canManageAnyOrganization(permissions)).toBe(false);
	});

	it("treats organization edit access as management", () => {
		const permissions = {
			...MockNoPermissions,
			editAnySettings: true,
		};

		expect(canViewAnyOrganization(permissions)).toBe(true);
		expect(canManageAnyOrganization(permissions)).toBe(true);
	});

	it("treats organization create access as management", () => {
		const permissions = {
			...MockNoPermissions,
			createOrganization: true,
		};

		expect(canViewAnyOrganization(permissions)).toBe(false);
		expect(canManageAnyOrganization(permissions)).toBe(true);
	});

	it("returns false without organization permissions", () => {
		expect(canViewAnyOrganization(MockNoPermissions)).toBe(false);
		expect(canManageAnyOrganization(MockNoPermissions)).toBe(false);
		expect(canManageAnyOrganization(MockPermissions)).toBe(true);
	});
});
