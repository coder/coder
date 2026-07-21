import { describe, expect, it } from "vitest";
import type { Organization } from "#/api/typesGenerated";
import { resolveAIResourceOrganization } from "./AIResourceOrganizationContext";

const organizations: Organization[] = [
	{
		id: "other-id",
		name: "other",
		display_name: "Other",
		description: "",
		icon: "",
		created_at: "",
		updated_at: "",
		is_default: false,
		default_org_member_roles: [],
	},
	{
		id: "default-id",
		name: "default",
		display_name: "Default",
		description: "",
		icon: "",
		created_at: "",
		updated_at: "",
		is_default: true,
		default_org_member_roles: [],
	},
];

describe("resolveAIResourceOrganization", () => {
	it("uses the URL organization when it is permitted", () => {
		expect(resolveAIResourceOrganization(organizations, "other")).toEqual(
			organizations[0],
		);
	});

	it("uses the permitted default for a missing or invalid URL organization", () => {
		expect(resolveAIResourceOrganization(organizations, null)).toEqual(
			organizations[1],
		);
		expect(resolveAIResourceOrganization(organizations, "invalid")).toEqual(
			organizations[1],
		);
	});

	it("uses the first permitted organization when none is default", () => {
		expect(
			resolveAIResourceOrganization(
				organizations.map((organization) => ({
					...organization,
					is_default: false,
				})),
				null,
			),
		).toMatchObject({ id: "other-id" });
	});

	it("returns undefined when no organization is permitted", () => {
		expect(resolveAIResourceOrganization([], "default")).toBeUndefined();
	});
});
