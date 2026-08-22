import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProvider,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	canManageProviderModels,
	deriveProviderStates,
	type ProviderState,
} from "./providerStates";

const baseProviderState: ProviderState = {
	key: MockChatModelProviderDescriptor.id,
	provider: MockChatModelProviderDescriptor.type,
	label: MockChatModelProviderDescriptor.display_name,
	providerDescriptor: MockChatModelProviderDescriptor,
	models: [],
	catalogModelCount: 0,
	hasEffectiveAPIKey: true,
	allowUserAPIKey: false,
};

describe("deriveProviderStates", () => {
	it("orders descriptors by display label", () => {
		const descriptors: TypesGen.ChatModelProviderDescriptor[] = [
			{
				...MockChatModelProviderDescriptor,
				id: "google-id",
				type: "google",
				display_name: "Google",
			},
			{
				...MockChatModelProviderDescriptor,
				id: "anthropic-id",
				type: "anthropic",
				display_name: "Anthropic",
			},
		];

		const states = deriveProviderStates([], descriptors, undefined);

		expect(states.map((state) => state.provider)).toEqual([
			"anthropic",
			"google",
		]);
	});

	it("matches model configs to descriptors by ai_provider_id", () => {
		const modelConfigs = [
			{ ...MockChatModel, id: "m1" },
			{ ...MockChatModel, id: "m2" },
		];

		const states = deriveProviderStates(
			modelConfigs,
			[MockChatModelProviderDescriptor],
			undefined,
		);

		expect(states[0].models.map((model) => model.id)).toEqual(["m1", "m2"]);
	});

	it("uses availability only for catalog model counts", () => {
		const availability: TypesGen.ChatModelAvailabilityResponse = {
			providers: [
				{
					...MockChatModelProvider,
					models: [
						{
							id: "openai:gpt-x",
							provider: "openai",
							model: "gpt-x",
							display_name: "GPT-X",
						},
					],
				},
			],
			unsupported_providers: [],
		};

		const states = deriveProviderStates(
			[],
			[MockChatModelProviderDescriptor],
			availability,
		);

		expect(states[0].catalogModelCount).toBe(1);
	});

	it("treats null provider models as an empty catalog", () => {
		const states = deriveProviderStates([], [MockChatModelProviderDescriptor], {
			providers: [{ ...MockChatModelProvider, models: null }],
			unsupported_providers: [],
		});

		expect(states[0].catalogModelCount).toBe(0);
	});

	it.each([
		{
			name: "uses ambient credentials",
			hasAPIKey: false,
			hasUserAPIKey: false,
			hasEffectiveAPIKey: true,
		},
		{
			name: "ignores an ineffective user key",
			hasAPIKey: false,
			hasUserAPIKey: true,
			hasEffectiveAPIKey: false,
		},
	])("$name", ({ hasAPIKey, hasUserAPIKey, hasEffectiveAPIKey }) => {
		const descriptor: TypesGen.ChatModelProviderDescriptor = {
			...MockChatModelProviderDescriptor,
			has_api_key: hasAPIKey,
			has_user_api_key: hasUserAPIKey,
			has_effective_api_key: hasEffectiveAPIKey,
		};

		const states = deriveProviderStates([], [descriptor], undefined);

		expect(states[0].hasEffectiveAPIKey).toBe(hasEffectiveAPIKey);
	});
});

describe("canManageProviderModels", () => {
	it("returns true when the provider has an effective API key", () => {
		expect(canManageProviderModels(baseProviderState)).toBe(true);
	});

	it("returns true when user-supplied API keys are allowed", () => {
		expect(
			canManageProviderModels({
				...baseProviderState,
				hasEffectiveAPIKey: false,
				allowUserAPIKey: true,
			}),
		).toBe(true);
	});

	it("returns false with no key and user keys disallowed", () => {
		expect(
			canManageProviderModels({
				...baseProviderState,
				hasEffectiveAPIKey: false,
				allowUserAPIKey: false,
			}),
		).toBe(false);
	});

	it("returns false when the provider is disabled", () => {
		expect(
			canManageProviderModels({
				...baseProviderState,
				providerDescriptor: {
					...MockChatModelProviderDescriptor,
					enabled: false,
				},
			}),
		).toBe(false);
	});

	it("returns false for undefined provider state", () => {
		expect(canManageProviderModels(undefined)).toBe(false);
	});
});
