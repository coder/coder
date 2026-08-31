import { describe, expect, it } from "vitest";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	organizationAddModelPath,
	selectModelOrganization,
} from "./organizationModels";

describe("selectModelOrganization", () => {
	const organizations = [MockDefaultOrganization, MockOrganization2];

	it("falls back only when no organization is requested", () => {
		expect(selectModelOrganization(organizations, null)).toEqual({
			organization: MockDefaultOrganization,
			requestedOrganizationDenied: false,
		});
		expect(
			selectModelOrganization(
				organizations.map((organization) => ({
					...organization,
					is_default: false,
				})),
				null,
			).organization?.id,
		).toBe(MockDefaultOrganization.id);
		expect(selectModelOrganization([], null)).toEqual({
			organization: undefined,
			requestedOrganizationDenied: false,
		});
	});

	it("uses exactly the requested accessible organization", () => {
		expect(
			selectModelOrganization(organizations, MockOrganization2.name),
		).toEqual({
			organization: MockOrganization2,
			requestedOrganizationDenied: false,
		});
	});

	it("marks a requested missing organization as denied", () => {
		expect(selectModelOrganization(organizations, "missing")).toEqual({
			organization: MockDefaultOrganization,
			requestedOrganizationDenied: true,
		});
	});

	it("preserves auxiliary parameters in organization model paths", () => {
		const params = new URLSearchParams({
			provider: "openai",
			duplicate: "model-id",
		});

		expect(organizationAddModelPath(MockOrganization2, params)).toBe(
			`/ai/settings/models/add?provider=openai&duplicate=model-id&org=${MockOrganization2.name}`,
		);
	});
});
