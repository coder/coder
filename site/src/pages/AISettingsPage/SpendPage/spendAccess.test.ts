import { expect, it } from "vitest";
import { withDefaultFeatures } from "#/api/api";
import { MockEntitlements, MockOrganization } from "#/testHelpers/entities";
import { canViewAISpend } from "./spendAccess";

const mockAIGatewayEntitlements = {
	...MockEntitlements,
	features: withDefaultFeatures({
		aibridge: { enabled: true, entitlement: "entitled" },
	}),
};

it("admits readers of at least one organization while AI Gateway is enabled", () => {
	expect(canViewAISpend(mockAIGatewayEntitlements, [MockOrganization])).toBe(
		true,
	);
});

it("denies access without a readable organization", () => {
	expect(canViewAISpend(mockAIGatewayEntitlements, [])).toBe(false);
	expect(canViewAISpend(mockAIGatewayEntitlements, undefined)).toBe(false);
});

it("denies access once AI Gateway is disabled despite cached organizations", () => {
	expect(
		canViewAISpend(
			{
				...mockAIGatewayEntitlements,
				features: {
					...mockAIGatewayEntitlements.features,
					aibridge: { enabled: false, entitlement: "entitled" },
				},
			},
			[MockOrganization],
		),
	).toBe(false);
});
