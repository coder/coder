import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
	MockCompactionChatModel,
} from "#/testHelpers/chatModels";
import {
	bindingCompactionTriggerPoint,
	bindingCompactionTriggerSource,
	compactionPointAsPercent,
	compactionThresholdLabel,
	isCompactionPointBeyondWindow,
	type OrganizationCompactionTrigger,
	type ResolvedCompactionThreshold,
	resolveChatCompactionThreshold,
	resolveCompactionTriggersByOrganization,
	resolveOrganizationCompactionTrigger,
} from "./compactionTriggers";
import { providerInfoByIDFromDescriptors } from "./utils/modelOptions";

const mockChatModel: TypesGen.ChatModel = {
	...MockChatModel,
	id: "chat-model",
	context_limit: 128_000,
	compression_threshold: 80,
};
const mockCompactionTrigger: OrganizationCompactionTrigger = {
	model: MockCompactionChatModel,
	trigger: { thresholdPercent: 50, contextLimit: 32_000 },
	pointTokens: 16_000,
};
const mockCompactionOverrides: TypesGen.ChatModelOverridesResponse = {
	overrides: [
		{ context: "compaction", model_config_id: MockCompactionChatModel.id },
	],
};
const providers = providerInfoByIDFromDescriptors([
	MockChatModelProviderDescriptor,
]);
const error = new Error("Network Error");

describe("compaction triggers", () => {
	it.each([
		[{ thresholdPercent: 0, contextLimit: 1 }, 0],
		[{ thresholdPercent: 99, contextLimit: 100 }, 99],
		[{ thresholdPercent: 80, contextLimit: 128_000 }, 102_400],
		[{ thresholdPercent: 100, contextLimit: 1 }, undefined],
		[{ thresholdPercent: -1, contextLimit: 1 }, undefined],
		[{ thresholdPercent: 50, contextLimit: 0 }, undefined],
		[{ thresholdPercent: 50, contextLimit: -1 }, undefined],
	])("reports the chat trigger point for %o", (chat, expected) => {
		expect(bindingCompactionTriggerPoint(chat, undefined)).toBe(expected);
	});

	it("converts compaction points to percentages of a known window", () => {
		expect(compactionPointAsPercent(32_000, 128_000)).toBe(25);
		expect(compactionPointAsPercent(32_000, 0)).toBeUndefined();
	});

	// Binding cases must match TestBindingCompactionTriggerSource in
	// coderd/x/chatd/generation_preparer_internal_test.go.
	it("selects the lower enabled point and prefers chat on ties", () => {
		const chat = { thresholdPercent: 80, contextLimit: 100_000 };

		expect(
			bindingCompactionTriggerSource(chat, {
				thresholdPercent: 50,
				contextLimit: 100_000,
			}),
		).toBe("organization");
		expect(
			bindingCompactionTriggerSource(chat, {
				thresholdPercent: 80,
				contextLimit: 100_000,
			}),
		).toBe("chat");
		expect(
			bindingCompactionTriggerSource(chat, {
				thresholdPercent: 100,
				contextLimit: 100_000,
			}),
		).toBe("chat");
		expect(
			bindingCompactionTriggerSource(chat, {
				thresholdPercent: 80,
				contextLimit: 0,
			}),
		).toBe("chat");
		expect(
			bindingCompactionTriggerSource(chat, {
				thresholdPercent: 0,
				contextLimit: 32_000,
			}),
		).toBe("organization");
		expect(
			bindingCompactionTriggerSource(
				{ thresholdPercent: 70, contextLimit: 100_000 },
				{ thresholdPercent: 95, contextLimit: 80_000 },
			),
		).toBe("chat");
	});

	it("uses the organization trigger when the chat trigger is disabled", () => {
		expect(
			bindingCompactionTriggerSource(
				{ thresholdPercent: 100, contextLimit: 100_000 },
				{ thresholdPercent: 80, contextLimit: 100_000 },
			),
		).toBe("organization");
		expect(
			bindingCompactionTriggerSource(
				{ thresholdPercent: 80, contextLimit: 0 },
				{ thresholdPercent: 80, contextLimit: 100_000 },
			),
		).toBe("organization");
		expect(
			bindingCompactionTriggerSource(
				{ thresholdPercent: 100, contextLimit: 100_000 },
				{ thresholdPercent: 100, contextLimit: 100_000 },
			),
		).toBe("chat");
	});

	it("reports the token point of whichever trigger binds", () => {
		const chat = { thresholdPercent: 80, contextLimit: 128_000 };

		expect(bindingCompactionTriggerPoint(chat, mockCompactionTrigger)).toBe(
			16_000,
		);
		expect(
			bindingCompactionTriggerPoint(
				{ thresholdPercent: 100, contextLimit: 128_000 },
				mockCompactionTrigger,
			),
		).toBe(16_000);
	});

	it("treats a point past a known window as beyond it, but not one at the edge", () => {
		expect(isCompactionPointBeyondWindow(16_000, 10_000)).toBe(true);
		expect(isCompactionPointBeyondWindow(10_000, 10_000)).toBe(false);
		expect(isCompactionPointBeyondWindow(9_000, 10_000)).toBe(false);
		expect(isCompactionPointBeyondWindow(16_000, 0)).toBe(false);
	});

	it("resolves an enabled member-visible organization override model", () => {
		expect(
			resolveOrganizationCompactionTrigger(
				MockCompactionChatModel.id,
				[MockCompactionChatModel],
				providers,
			),
		).toEqual(mockCompactionTrigger);
		expect(
			resolveOrganizationCompactionTrigger(
				undefined,
				[MockCompactionChatModel],
				providers,
			),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				MockCompactionChatModel.id,
				[],
				providers,
			),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				MockCompactionChatModel.id,
				[{ ...MockCompactionChatModel, enabled: false }],
				providers,
			),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				MockCompactionChatModel.id,
				[{ ...MockCompactionChatModel, compression_threshold: 100 }],
				providers,
			),
		).toBeUndefined();
	});

	it("ignores an organization override model whose provider is disabled", () => {
		const disabledProviders = providerInfoByIDFromDescriptors([
			{ ...MockChatModelProviderDescriptor, enabled: false },
		]);

		expect(
			resolveOrganizationCompactionTrigger(
				MockCompactionChatModel.id,
				[MockCompactionChatModel],
				disabledProviders,
			),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				MockCompactionChatModel.id,
				[MockCompactionChatModel],
				disabledProviders,
				"organization",
			),
		).toBeUndefined();
	});

	it("ignores an override the viewer lacks provider credentials for", () => {
		const unavailableProviders = providerInfoByIDFromDescriptors([
			{
				...MockChatModelProviderDescriptor,
				available: false,
				unavailable_reason: "user_api_key_required",
			},
		]);

		expect(
			resolveOrganizationCompactionTrigger(
				MockCompactionChatModel.id,
				[MockCompactionChatModel],
				unavailableProviders,
			),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				MockCompactionChatModel.id,
				[MockCompactionChatModel],
				unavailableProviders,
				"organization",
			),
		).toEqual(mockCompactionTrigger);
	});

	describe("resolveCompactionTriggersByOrganization", () => {
		const organizationID = MockCompactionChatModel.organization_id;

		it("reports an organization whose first overrides load failed", () => {
			expect(
				resolveCompactionTriggersByOrganization(
					[{ organizationID, data: undefined, error }],
					[MockCompactionChatModel],
					providers,
				),
			).toEqual({
				triggersByOrganizationID: new Map(),
				loadErrors: [{ organizationID, error }],
			});
		});

		it("keeps cached overrides without reporting a failed refetch", () => {
			expect(
				resolveCompactionTriggersByOrganization(
					[{ organizationID, data: mockCompactionOverrides, error }],
					[MockCompactionChatModel],
					providers,
				),
			).toEqual({
				triggersByOrganizationID: new Map([
					[organizationID, mockCompactionTrigger],
				]),
				loadErrors: [],
			});
		});

		it("reports nothing when the overrides load succeeds", () => {
			expect(
				resolveCompactionTriggersByOrganization(
					[{ organizationID, data: mockCompactionOverrides, error: null }],
					[MockCompactionChatModel],
					providers,
				).loadErrors,
			).toEqual([]);
		});

		it("ignores a failed load for an organization without enabled models", () => {
			expect(
				resolveCompactionTriggersByOrganization(
					[{ organizationID, data: undefined, error }],
					[{ ...MockCompactionChatModel, enabled: false }],
					providers,
				).loadErrors,
			).toEqual([]);
		});
	});

	describe("resolveChatCompactionThreshold", () => {
		const resolve = (
			overrides: Parameters<typeof resolveChatCompactionThreshold>[4],
			{
				models = [mockChatModel, MockCompactionChatModel],
				userThresholds,
			}: {
				models?: readonly TypesGen.ChatModel[];
				userThresholds?: readonly TypesGen.UserChatCompactionThreshold[];
			} = {},
		) =>
			resolveChatCompactionThreshold(
				mockChatModel.id,
				userThresholds,
				models,
				providers,
				overrides,
			);

		it("returns the organization point when its trigger binds", () => {
			expect(resolve({ data: mockCompactionOverrides, error: null })).toEqual({
				source: "organization",
				pointTokens: 16_000,
			});
		});

		it("keeps the user threshold when the organization point is higher", () => {
			expect(
				resolve(
					{ data: mockCompactionOverrides, error: null },
					{
						models: [
							mockChatModel,
							{
								...MockCompactionChatModel,
								context_limit: 128_000,
								compression_threshold: 90,
							},
						],
						userThresholds: [
							{ model_config_id: mockChatModel.id, threshold_percent: 60 },
						],
					},
				),
			).toEqual({
				percent: 60,
				source: "user",
				organizationOverrideNotLoaded: false,
			});
		});

		it("reports the binding organization trigger when the chat threshold is disabled", () => {
			expect(
				resolve(
					{ data: mockCompactionOverrides, error: null },
					{
						models: [
							mockChatModel,
							{ ...MockCompactionChatModel, context_limit: 256_000 },
						],
						userThresholds: [
							{ model_config_id: mockChatModel.id, threshold_percent: 100 },
						],
					},
				),
			).toEqual({
				source: "organization",
				pointTokens: 128_000,
			});
		});

		it("returns undefined for unknown or unloaded models", () => {
			expect(
				resolve(
					{ data: mockCompactionOverrides, error: null },
					{ models: [MockCompactionChatModel] },
				),
			).toBeUndefined();
			expect(
				resolveChatCompactionThreshold(
					mockChatModel.id,
					undefined,
					undefined,
					providers,
					{ data: mockCompactionOverrides, error: null },
				),
			).toBeUndefined();
		});

		it("flags the chat threshold when the first overrides load failed", () => {
			expect(resolve({ data: undefined, error })).toEqual({
				percent: 80,
				source: "model",
				organizationOverrideNotLoaded: true,
			});
		});

		it("does not flag a failed refetch with cached overrides", () => {
			expect(resolve({ data: { overrides: [] }, error })).toEqual({
				percent: 80,
				source: "model",
				organizationOverrideNotLoaded: false,
			});
			expect(resolve({ data: mockCompactionOverrides, error })).toEqual({
				source: "organization",
				pointTokens: 16_000,
			});
		});

		it("does not flag loaded overrides without a compaction override", () => {
			expect(resolve({ data: { overrides: [] }, error: null })).toEqual({
				percent: 80,
				source: "model",
				organizationOverrideNotLoaded: false,
			});
		});

		it("does not flag a pending overrides load", () => {
			expect(resolve({ data: undefined, error: null })).toEqual({
				percent: 80,
				source: "model",
				organizationOverrideNotLoaded: false,
			});
		});
	});

	it.each<[ResolvedCompactionThreshold, number, string | undefined]>([
		[
			{ source: "organization", pointTokens: 16_000 },
			200_000,
			"Compacts at 8% (organization override)",
		],
		[
			{ source: "organization", pointTokens: 10_000 },
			10_000,
			"Compacts at 100% (organization override)",
		],
		[{ source: "organization", pointTokens: 16_000 }, 10_000, undefined],
		[
			{ percent: 70, source: "model", organizationOverrideNotLoaded: false },
			200_000,
			"Compacts at 70%",
		],
		[
			{ percent: 33.33, source: "user", organizationOverrideNotLoaded: false },
			200_000,
			"Compacts at 33.3%",
		],
		[
			{ percent: 70, source: "model", organizationOverrideNotLoaded: true },
			200_000,
			"Compacts at 70% (organization override not loaded)",
		],
		[
			{ percent: 100, source: "user", organizationOverrideNotLoaded: false },
			200_000,
			undefined,
		],
		[
			{ percent: 100, source: "user", organizationOverrideNotLoaded: true },
			200_000,
			"Compaction off (organization override not loaded)",
		],
	])("labels %o in a %i-token window", (compaction, contextLimit, expected) => {
		expect(compactionThresholdLabel(compaction, contextLimit)).toBe(expected);
	});
});
