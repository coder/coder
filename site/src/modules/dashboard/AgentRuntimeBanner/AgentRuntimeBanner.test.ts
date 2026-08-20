import type { Feature } from "#/api/typesGenerated";
import { agentRuntimeBannerMessage } from "./AgentRuntimeBanner";

const entitledFeature = (overrides: Partial<Feature>): Feature => ({
	enabled: true,
	entitlement: "entitled",
	...overrides,
});

describe(agentRuntimeBannerMessage.name, () => {
	it("returns nothing without the feature", () => {
		expect(agentRuntimeBannerMessage(undefined)).toBeNull();
	});

	it("returns nothing when not entitled", () => {
		expect(
			agentRuntimeBannerMessage({
				enabled: false,
				entitlement: "not_entitled",
				actual: 100,
				limit: 100,
			}),
		).toBeNull();
	});

	it("returns nothing without an allocation", () => {
		expect(
			agentRuntimeBannerMessage(entitledFeature({ actual: 100 })),
		).toBeNull();
	});

	// A zero allocation means the feature is disabled, not exhausted.
	it("returns nothing for a zero allocation", () => {
		expect(
			agentRuntimeBannerMessage(entitledFeature({ actual: 50, limit: 0 })),
		).toBeNull();
	});

	it("returns nothing without a usage measurement", () => {
		expect(
			agentRuntimeBannerMessage(entitledFeature({ limit: 100 })),
		).toBeNull();
	});

	it("returns nothing below the allocation", () => {
		expect(
			agentRuntimeBannerMessage(entitledFeature({ actual: 99, limit: 100 })),
		).toBeNull();
	});

	it("reports the allocation from the first hour at it", () => {
		expect(
			agentRuntimeBannerMessage(entitledFeature({ actual: 100, limit: 100 })),
		).toBe(
			"Your deployment has used 100 of the 100 Coder Agent runtime hours included in the current license term. Contact your deployment administrator.",
		);
	});

	it("keeps the allocation message while under the hard limit", () => {
		expect(
			agentRuntimeBannerMessage(
				entitledFeature({ actual: 110, limit: 100, hard_limit: 120 }),
			),
		).toBe(
			"Your deployment has used 110 of the 100 Coder Agent runtime hours included in the current license term. Contact your deployment administrator.",
		);
	});

	it("reports the hard limit from the first hour at it", () => {
		expect(
			agentRuntimeBannerMessage(
				entitledFeature({ actual: 120, limit: 100, hard_limit: 120 }),
			),
		).toBe(
			"Your deployment has used 120 of the 100 Coder Agent runtime hours included in the current license term, reaching the hard limit of 120 hours. Contact your deployment administrator.",
		);
	});

	// The advisory soft limit is admin-facing only; see LicenseBanner.
	it("ignores the soft limit", () => {
		expect(
			agentRuntimeBannerMessage(
				entitledFeature({ actual: 90, limit: 100, soft_limit: 80 }),
			),
		).toBeNull();
	});

	it("derives a message during the grace period", () => {
		expect(
			agentRuntimeBannerMessage({
				enabled: true,
				entitlement: "grace_period",
				actual: 100,
				limit: 100,
			}),
		).toBe(
			"Your deployment has used 100 of the 100 Coder Agent runtime hours included in the current license term. Contact your deployment administrator.",
		);
	});
});
