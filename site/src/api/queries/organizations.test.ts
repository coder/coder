import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { AuthorizationCheck, Organization } from "#/api/typesGenerated";
import { permittedOrganizations } from "./organizations";

// Mock the API module
vi.mock("#/api/api", () => ({
	API: {
		getOrganizations: vi.fn(),
		checkAuthorization: vi.fn(),
	},
}));

const MockOrg1: Organization = {
	id: "org-1",
	name: "org-one",
	display_name: "Org One",
	description: "",
	icon: "",
	created_at: "",
	updated_at: "",
	is_default: true,
};

const MockOrg2: Organization = {
	id: "org-2",
	name: "org-two",
	display_name: "Org Two",
	description: "",
	icon: "",
	created_at: "",
	updated_at: "",
	is_default: false,
};

const templateCreateCheck: AuthorizationCheck = {
	object: { resource_type: "template" },
	action: "create",
};

describe("permittedOrganizations", () => {
	it("returns query config with correct queryKey", () => {
		const config = permittedOrganizations(templateCreateCheck);
		expect(config.queryKey).toEqual([
			"organizations",
			"permitted",
			templateCreateCheck,
		]);
	});

	it("fetches orgs and filters by permission check", async () => {
		const getOrgsMock = vi.mocked(API.getOrganizations);
		const checkAuthMock = vi.mocked(API.checkAuthorization);

		getOrgsMock.mockResolvedValue([MockOrg1, MockOrg2]);
		checkAuthMock.mockResolvedValue({
			"org-1": true,
			"org-2": false,
		});

		const config = permittedOrganizations(templateCreateCheck);
		const result = await config.queryFn!();

		// Should only return org-1 (which passed the check)
		expect(result).toEqual([MockOrg1]);

		// Verify the auth check was called with per-org checks
		expect(checkAuthMock).toHaveBeenCalledWith({
			checks: {
				"org-1": {
					...templateCreateCheck,
					object: {
						...templateCreateCheck.object,
						organization_id: "org-1",
					},
				},
				"org-2": {
					...templateCreateCheck,
					object: {
						...templateCreateCheck.object,
						organization_id: "org-2",
					},
				},
			},
		});
	});

	it("returns all orgs when all pass the check", async () => {
		const getOrgsMock = vi.mocked(API.getOrganizations);
		const checkAuthMock = vi.mocked(API.checkAuthorization);

		getOrgsMock.mockResolvedValue([MockOrg1, MockOrg2]);
		checkAuthMock.mockResolvedValue({
			"org-1": true,
			"org-2": true,
		});

		const config = permittedOrganizations(templateCreateCheck);
		const result = await config.queryFn!();

		expect(result).toEqual([MockOrg1, MockOrg2]);
	});

	it("returns empty array when no orgs pass the check", async () => {
		const getOrgsMock = vi.mocked(API.getOrganizations);
		const checkAuthMock = vi.mocked(API.checkAuthorization);

		getOrgsMock.mockResolvedValue([MockOrg1, MockOrg2]);
		checkAuthMock.mockResolvedValue({
			"org-1": false,
			"org-2": false,
		});

		const config = permittedOrganizations(templateCreateCheck);
		const result = await config.queryFn!();

		expect(result).toEqual([]);
	});
});
