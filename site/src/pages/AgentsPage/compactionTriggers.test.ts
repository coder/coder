import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	bindingCompactionTriggerPoint,
	bindingCompactionTriggerSource,
	compactionPointAsPercent,
	compactionTriggerPoint,
	isCompactionPointBeyondWindow,
	isCompactionTriggerEnabled,
	organizationCompactionTrigger,
	resolveCompactionThreshold,
	resolveCompactionTriggersByOrganization,
	resolveOrganizationCompactionTrigger,
} from "./compactionTriggers";
import { providerInfoByIDFromDescriptors } from "./utils/modelOptions";

describe("compaction triggers", () => {
	it("enables thresholds from 0 through 99 with a positive context limit", () => {
		expect(
			isCompactionTriggerEnabled({ thresholdPercent: 0, contextLimit: 1 }),
		).toBe(true);
		expect(
			isCompactionTriggerEnabled({ thresholdPercent: 99, contextLimit: 1 }),
		).toBe(true);
		expect(
			isCompactionTriggerEnabled({ thresholdPercent: 100, contextLimit: 1 }),
		).toBe(false);
		expect(
			isCompactionTriggerEnabled({ thresholdPercent: -1, contextLimit: 1 }),
		).toBe(false);
		expect(
			isCompactionTriggerEnabled({ thresholdPercent: 50, contextLimit: 0 }),
		).toBe(false);
		expect(
			isCompactionTriggerEnabled({ thresholdPercent: 50, contextLimit: -1 }),
		).toBe(false);
	});

	it("computes compaction trigger token counts and percentages", () => {
		expect(
			compactionTriggerPoint({
				thresholdPercent: 80,
				contextLimit: 128_000,
			}),
		).toBe(102_400);
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
		const organizationTrigger = organizationCompactionTrigger({
			...MockChatModel,
			compression_threshold: 50,
			context_limit: 32_000,
		});

		expect(bindingCompactionTriggerPoint(chat, undefined)).toBe(102_400);
		expect(bindingCompactionTriggerPoint(chat, organizationTrigger)).toBe(
			16_000,
		);
		expect(
			bindingCompactionTriggerPoint(
				{ thresholdPercent: 100, contextLimit: 128_000 },
				organizationTrigger,
			),
		).toBe(16_000);
		expect(
			bindingCompactionTriggerPoint(
				{ thresholdPercent: 100, contextLimit: 128_000 },
				undefined,
			),
		).toBeUndefined();
		expect(
			bindingCompactionTriggerPoint(
				{ thresholdPercent: 80, contextLimit: 0 },
				undefined,
			),
		).toBeUndefined();
	});

	it("treats a point past a known window as beyond it, but not one at the edge", () => {
		expect(isCompactionPointBeyondWindow(16_000, 10_000)).toBe(true);
		expect(isCompactionPointBeyondWindow(10_000, 10_000)).toBe(false);
		expect(isCompactionPointBeyondWindow(9_000, 10_000)).toBe(false);
		expect(isCompactionPointBeyondWindow(16_000, 0)).toBe(false);
	});

	it("resolves an enabled member-visible organization override model", () => {
		const model: TypesGen.ChatModel = {
			...MockChatModel,
			id: "compaction-model",
			context_limit: 40_000,
			compression_threshold: 50,
		};
		const providers = providerInfoByIDFromDescriptors([
			MockChatModelProviderDescriptor,
		]);

		expect(
			resolveOrganizationCompactionTrigger(model.id, [model], providers),
		).toEqual({
			model,
			trigger: { thresholdPercent: 50, contextLimit: 40_000 },
			pointTokens: 20_000,
		});
		expect(
			resolveOrganizationCompactionTrigger(undefined, [model], providers),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(model.id, [], providers),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				model.id,
				[{ ...model, enabled: false }],
				providers,
			),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				model.id,
				[{ ...model, compression_threshold: 100 }],
				providers,
			),
		).toBeUndefined();
	});

	it("ignores an organization override model whose provider is disabled", () => {
		const model: TypesGen.ChatModel = {
			...MockChatModel,
			id: "compaction-model",
			context_limit: 40_000,
			compression_threshold: 50,
		};
		const providers = providerInfoByIDFromDescriptors([
			{ ...MockChatModelProviderDescriptor, enabled: false },
		]);

		expect(
			resolveOrganizationCompactionTrigger(model.id, [model], providers),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				model.id,
				[model],
				providers,
				"organization",
			),
		).toBeUndefined();
	});

	it("ignores an override the viewer lacks provider credentials for", () => {
		const model: TypesGen.ChatModel = {
			...MockChatModel,
			id: "compaction-model",
			context_limit: 40_000,
			compression_threshold: 50,
		};
		const providers = providerInfoByIDFromDescriptors([
			{
				...MockChatModelProviderDescriptor,
				available: false,
				unavailable_reason: "user_api_key_required",
			},
		]);

		expect(
			resolveOrganizationCompactionTrigger(model.id, [model], providers),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				model.id,
				[model],
				providers,
				"organization",
			),
		).toMatchObject({ model, pointTokens: 20_000 });
	});

	describe("resolveCompactionTriggersByOrganization", () => {
		const compactionModel: TypesGen.ChatModel = {
			...MockChatModel,
			id: "compaction-model",
			context_limit: 40_000,
			compression_threshold: 50,
		};
		const overrides: TypesGen.ChatModelOverridesResponse = {
			overrides: [
				{ context: "compaction", model_config_id: compactionModel.id },
			],
		};
		const providers = providerInfoByIDFromDescriptors([
			MockChatModelProviderDescriptor,
		]);
		const error = new Error("Network Error");
		const organizationID = compactionModel.organization_id;

		it("reports an organization whose first overrides load failed", () => {
			expect(
				resolveCompactionTriggersByOrganization(
					[{ organizationID, data: undefined, error }],
					[compactionModel],
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
					[{ organizationID, data: overrides, error }],
					[compactionModel],
					providers,
				),
			).toEqual({
				triggersByOrganizationID: new Map([
					[organizationID, organizationCompactionTrigger(compactionModel)],
				]),
				loadErrors: [],
			});
		});

		it("reports nothing when the overrides load succeeds", () => {
			expect(
				resolveCompactionTriggersByOrganization(
					[{ organizationID, data: overrides, error: null }],
					[compactionModel],
					providers,
				).loadErrors,
			).toEqual([]);
		});

		it("ignores a failed load for an organization without enabled models", () => {
			expect(
				resolveCompactionTriggersByOrganization(
					[{ organizationID, data: undefined, error }],
					[{ ...compactionModel, enabled: false }],
					providers,
				).loadErrors,
			).toEqual([]);
		});
	});

	describe("resolveCompactionThreshold", () => {
		const chatModel: TypesGen.ChatModel = {
			...MockChatModel,
			id: "chat-model",
			context_limit: 128_000,
			compression_threshold: 80,
		};
		const organizationTrigger = (
			thresholdPercent: number,
			contextLimit: number,
		) =>
			organizationCompactionTrigger({
				...MockChatModel,
				id: "compaction-model",
				compression_threshold: thresholdPercent,
				context_limit: contextLimit,
			});

		it("returns the organization percent when its trigger binds", () => {
			expect(
				resolveCompactionThreshold(
					chatModel.id,
					undefined,
					[chatModel],
					organizationTrigger(50, 32_000),
				),
			).toEqual({
				percent: 12.5,
				source: "organization",
				pointTokens: 16_000,
			});
		});

		it("keeps the user threshold when the organization point is higher", () => {
			expect(
				resolveCompactionThreshold(
					chatModel.id,
					[{ model_config_id: chatModel.id, threshold_percent: 60 }],
					[chatModel],
					organizationTrigger(90, 128_000),
				),
			).toEqual({ percent: 60, source: "user" });
		});

		it("reports the binding organization trigger when the chat threshold is disabled", () => {
			expect(
				resolveCompactionThreshold(
					chatModel.id,
					[{ model_config_id: chatModel.id, threshold_percent: 100 }],
					[chatModel],
					organizationTrigger(50, 256_000),
				),
			).toEqual({
				percent: 100,
				source: "organization",
				pointTokens: 128_000,
			});
		});

		it("falls back to the model default without user or organization input", () => {
			expect(
				resolveCompactionThreshold(
					chatModel.id,
					undefined,
					[chatModel],
					undefined,
				),
			).toEqual({ percent: 80, source: "model" });
		});

		it("returns undefined for unknown models", () => {
			expect(
				resolveCompactionThreshold(
					"missing",
					undefined,
					[chatModel],
					organizationTrigger(50, 32_000),
				),
			).toBeUndefined();
		});
	});
});
