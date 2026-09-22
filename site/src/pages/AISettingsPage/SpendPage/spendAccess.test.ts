import { expect, it } from "vitest";
import { withDefaultFeatures } from "#/api/api";
import { MockEntitlements, MockOrganization } from "#/testHelpers/entities";
import { canViewAISpend } from "./spendAccess";

const aiGatewayEntitlements = (enabled: boolean) => ({
	...MockEntitlements,
	features: withDefaultFeatures({
		aibridge: { enabled, entitlement: "entitled" },
	}),
});

it("admits readers of at least one organization while AI Gateway is enabled", () => {
	expect(canViewAISpend(aiGatewayEntitlements(true), [MockOrganization])).toBe(
		true,
	);
});

it("denies access without a readable organization", () => {
	expect(canViewAISpend(aiGatewayEntitlements(true), [])).toBe(false);
	expect(canViewAISpend(aiGatewayEntitlements(true), undefined)).toBe(false);
});

it("denies access once AI Gateway is disabled despite cached organizations", () => {
	expect(canViewAISpend(aiGatewayEntitlements(false), [MockOrganization])).toBe(
		false,
	);
});
