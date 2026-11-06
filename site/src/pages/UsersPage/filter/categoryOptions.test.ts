import { describe, expect, it, vi } from "vitest";
import type { Role } from "#/api/typesGenerated";
import type { FilterOption } from "#/components/Filter/FilterCombobox/types";
import { MockOwnerRole } from "#/testHelpers/entities";
import {
	getRoleFilterOptions,
	getStatusFilterOptions,
	getUserTypeFilterOptions,
} from "./categoryOptions";

const labelsAndValues = (options: readonly FilterOption[]) =>
	options.map(({ label, value }) => ({ label, value }));

const queryClientWithRoles = (roles: readonly Role[]) => ({
	fetchQuery: vi.fn().mockResolvedValue(roles),
});

describe("getStatusFilterOptions", () => {
	it("returns every status for an empty query", async () => {
		expect(labelsAndValues(await getStatusFilterOptions(""))).toEqual([
			{ label: "Active", value: "active" },
			{ label: "Dormant", value: "dormant" },
			{ label: "Suspended", value: "suspended" },
		]);
	});

	it("matches the query ignoring case and surrounding whitespace", async () => {
		expect(labelsAndValues(await getStatusFilterOptions("  ACT "))).toEqual([
			{ label: "Active", value: "active" },
		]);
	});

	it("returns no options when nothing matches", async () => {
		expect(await getStatusFilterOptions("missing")).toEqual([]);
	});
});

describe("getRoleFilterOptions", () => {
	it("labels roles by display name, falling back to the name", async () => {
		const queryClient = queryClientWithRoles([
			MockOwnerRole,
			{ ...MockOwnerRole, name: "auditor", display_name: "" },
		]);

		expect(await getRoleFilterOptions("", queryClient)).toEqual([
			{ label: "Owner", value: "owner" },
			{ label: "auditor", value: "auditor" },
		]);
	});

	it("matches the query against the role name as well as the label", async () => {
		const queryClient = queryClientWithRoles([
			{ ...MockOwnerRole, name: "template-admin", display_name: "Templates" },
			MockOwnerRole,
		]);

		expect(await getRoleFilterOptions("admin", queryClient)).toEqual([
			{ label: "Templates", value: "template-admin" },
		]);
	});
});

describe("getUserTypeFilterOptions", () => {
	it("returns the service account option with its chip token", async () => {
		expect(await getUserTypeFilterOptions("service")).toEqual([
			{
				label: "Service account",
				value: "service_account",
				token: "service_account:true",
			},
		]);
	});
});
