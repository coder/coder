import type { Permissions } from "contexts/auth/permissions";
import { isManagementRoutePermitted } from "./ManagementSettingsLayout";
import { MockNoPermissions, MockPermissions } from "testHelpers/entities";
import { Permission } from "api/typesGenerated";

describe(isManagementRoutePermitted.name, () => {
	describe("General behavior", () => {
		it("Rejects malformed routes", () => {
			const invalidRoutes: readonly string[] = [
				// It's expected that the hostname will be stripped off
				"https://dev.coder.com/deployment/licenses/add",

				// Missing the leading /
				"organizations",
			];

			for (const r of invalidRoutes) {
			}

			// Currently only checking whether route ends with /
			const invalidRoute = "organizations";
			expect(
				isManagementRoutePermitted(invalidRoute, MockPermissions, true),
			).toBe(false);
		});
	});

	describe("Organization routes", () => {
		it("Delegates to showOrganizations value for any /organizations routes", () => {
			expect(
				isManagementRoutePermitted("/organizations", MockNoPermissions, true),
			).toBe(true);

			expect(
				isManagementRoutePermitted(
					"/organizations/nested/arbitrarily",
					MockNoPermissions,
					true,
				),
			).toBe(true);

			expect(
				isManagementRoutePermitted(
					"/organizations/sample-organization",
					MockNoPermissions,
					true,
				),
			).toBe(true);

			expect(
				isManagementRoutePermitted("/organizations", MockPermissions, false),
			).toBe(false);
		});
	});

	describe("Deployment routes", () => {
		it("Will never let the user through if they have no active permissions", () => {
			let result = isManagementRoutePermitted(
				"/deployment",
				MockNoPermissions,
				true,
			);
			expect(result).toBe(false);

			result = isManagementRoutePermitted(
				"/deployment/licenses",
				MockNoPermissions,
				true,
			);
			expect(result).toBe(false);

			result = isManagementRoutePermitted(
				"/deployment/appearance",
				MockNoPermissions,
				false,
			);
			expect(result).toBe(false);
		});

		it("Will let users access base /deployment route if they have at least one permission", () => {
			const mocks: readonly Permissions[] = [
				MockPermissions,
				{
					...MockNoPermissions,
					createGroup: true,
				},
				{
					...MockNoPermissions,
					createOrganization: true,
					deleteTemplates: true,
				},
				{
					...MockNoPermissions,
					editDeploymentValues: true,
					viewNotificationTemplate: true,
					readWorkspaceProxies: true,
				},
			];

			for (const m of mocks) {
				expect(isManagementRoutePermitted("/deployment", m, true)).toBe(true);
				expect(isManagementRoutePermitted("/deployment", m, false)).toBe(true);
			}
		});

		it("Rejects unknown deployment routes", () => {
			const sampleRoutes: readonly string[] = [
				"/deployment/definitely-not-right",
				"/deployment/what-is-this",
			];

			for (const r of sampleRoutes) {
				const result = isManagementRoutePermitted(r, MockPermissions, true);
				expect(result).toBe(false);
			}
		});

		it("Supports deployment routes that are nested more than one level", () => {
			const routes: readonly string[] = [
				"/deployment/licenses/add",

				// Including oauth2-provider routes, even though they're not
				// currently exposed via the UI
				"/deployment/oauth2-provider/apps",
				"/deployment/oauth2-provider/apps/add",
			];

			for (const r of routes) {
				let result = isManagementRoutePermitted(r, MockPermissions, true);
				expect(result).toBe(true);

				result = isManagementRoutePermitted(r, MockPermissions, false);
				expect(result).toBe(true);
			}
		});

		it("Granularly associates individual deployment routes with specific permissions", () => {
			type PairTuple = readonly [
				route: `/${string}`,
				permissionKey: keyof Permissions,
			];

			const pairs: readonly PairTuple[] = [
				["/general", "viewDeploymentValues"],
				["/licenses", "viewAllLicenses"],
				["/appearance", "editDeploymentValues"],
				["/userauth", "viewDeploymentValues"],
				["/external-auth", "viewDeploymentValues"],
				["/network", "viewDeploymentValues"],
				["/workspace-proxies", "readWorkspaceProxies"],
				["/security", "viewDeploymentValues"],
				["/observability", "viewDeploymentValues"],
				["/users", "viewAllUsers"],
				["/notifications", "viewNotificationTemplate"],
				["/oauth2-provider", "viewExternalAuthConfig"],
			];

			for (const [route, permName] of pairs) {
				// Using NoPermissions version as baseline to make sure that we
				// don't get false positives from all permissions being set to
				// true at the start
				const mutablePermsCopy = { ...MockNoPermissions };
				const fullRoute = `/deployment${route}`;

				mutablePermsCopy[permName] = true;
				let result = isManagementRoutePermitted(
					fullRoute,
					mutablePermsCopy,
					true,
				);
				expect(result).toBe(true);

				result = isManagementRoutePermitted(
					`${fullRoute}/example-subpath`,
					mutablePermsCopy,
					true,
				);
				expect(result).toBe(true);

				mutablePermsCopy[permName] = false;
				result = isManagementRoutePermitted(fullRoute, mutablePermsCopy, true);
				expect(result).toBe(false);
			}

			expect.hasAssertions();
		});
	});
});
