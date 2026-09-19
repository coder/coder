import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	bindingCompactionTrigger,
	bindingCompactionTriggerPoint,
	compactionPointAsPercent,
	compactionTriggerPoint,
	isCompactionTriggerEnabled,
	resolveCompactionThreshold,
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

	it("selects the lower enabled point and prefers chat on ties", () => {
		const chat = { thresholdPercent: 80, contextLimit: 100_000 };

		expect(
			bindingCompactionTrigger(chat, {
				thresholdPercent: 50,
				contextLimit: 100_000,
			}),
		).toBe("organization");
		expect(
			bindingCompactionTrigger(chat, {
				thresholdPercent: 80,
				contextLimit: 100_000,
			}),
		).toBe("chat");
		expect(
			bindingCompactionTrigger(chat, {
				thresholdPercent: 100,
				contextLimit: 100_000,
			}),
		).toBe("chat");
	});

	it("uses the organization trigger when the chat trigger is disabled", () => {
		expect(
			bindingCompactionTrigger(
				{ thresholdPercent: 100, contextLimit: 100_000 },
				{ thresholdPercent: 80, contextLimit: 100_000 },
			),
		).toBe("organization");
		expect(
			bindingCompactionTrigger(
				{ thresholdPercent: 100, contextLimit: 100_000 },
				{ thresholdPercent: 100, contextLimit: 100_000 },
			),
		).toBe("chat");
	});

	it("reports the token point of whichever trigger binds", () => {
		const chat = { thresholdPercent: 80, contextLimit: 128_000 };
		const organizationTrigger = {
			model: MockChatModel,
			trigger: { thresholdPercent: 50, contextLimit: 32_000 },
			point: 16_000,
		};

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

	it("resolves an enabled member-visible organization override model", () => {
		const model: TypesGen.ChatModel = {
			...MockChatModel,
			id: "compaction-model",
			context_limit: 40_000,
			compression_threshold: 50,
		};
		const overrides: readonly TypesGen.ChatModelOverrideResponse[] = [
			{ context: "compaction", model_config_id: model.id },
		];
		const providers = providerInfoByIDFromDescriptors([
			MockChatModelProviderDescriptor,
		]);

		expect(
			resolveOrganizationCompactionTrigger(overrides, [model], providers),
		).toEqual({
			model,
			trigger: { thresholdPercent: 50, contextLimit: 40_000 },
			point: 20_000,
		});
		expect(
			resolveOrganizationCompactionTrigger(overrides, [], providers),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				overrides,
				[{ ...model, enabled: false }],
				providers,
			),
		).toBeUndefined();
		expect(
			resolveOrganizationCompactionTrigger(
				overrides,
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
		const overrides: readonly TypesGen.ChatModelOverrideResponse[] = [
			{ context: "compaction", model_config_id: model.id },
		];

		expect(
			resolveOrganizationCompactionTrigger(
				overrides,
				[model],
				providerInfoByIDFromDescriptors([
					{ ...MockChatModelProviderDescriptor, enabled: false },
				]),
			),
		).toBeUndefined();
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
		) => ({
			model: { ...MockChatModel, id: "compaction-model" },
			trigger: { thresholdPercent, contextLimit },
			point: (contextLimit * thresholdPercent) / 100,
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
