import { MockPermissions } from "#/testHelpers/entities";
import { canViewTemplates, canViewWorkspaces, type Permissions } from ".";

const permissionsWithout = (key: keyof Permissions): Permissions => {
	const { [key]: _omitted, ...rest } = MockPermissions;
	return rest as Permissions;
};

describe("canViewWorkspaces", () => {
	it.each([
		[true, true],
		[false, false],
	])("returns %s when viewWorkspaces is %s", (expected, value) => {
		expect(
			canViewWorkspaces({ ...MockPermissions, viewWorkspaces: value }),
		).toBe(expected);
	});

	it("returns true when the check is absent", () => {
		expect(canViewWorkspaces(permissionsWithout("viewWorkspaces"))).toBe(true);
	});
});

describe("canViewTemplates", () => {
	it.each([
		[true, true, true],
		[true, true, false],
		[true, false, true],
		[false, false, false],
	])("returns %s when viewTemplates is %s and viewWorkspaces is %s", (expected, viewTemplates, viewWorkspaces) => {
		expect(
			canViewTemplates({
				...MockPermissions,
				viewTemplates,
				viewWorkspaces,
			}),
		).toBe(expected);
	});

	it("returns true when either check is absent", () => {
		expect(canViewTemplates(permissionsWithout("viewTemplates"))).toBe(true);
		expect(
			canViewTemplates({
				...permissionsWithout("viewTemplates"),
				viewWorkspaces: false,
			}),
		).toBe(true);
		expect(
			canViewTemplates({
				...permissionsWithout("viewWorkspaces"),
				viewTemplates: false,
			}),
		).toBe(true);
	});
});
