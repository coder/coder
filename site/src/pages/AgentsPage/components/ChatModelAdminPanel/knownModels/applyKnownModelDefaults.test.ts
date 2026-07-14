import { describe, expect, it } from "vitest";
import { buildInitialModelFormValues } from "../modelConfigFormLogic";
import { pricingFieldNameList } from "../pricingFields";
import {
	type ApplyKnownModelDefaultsParameters,
	type ApplyKnownModelDefaultsResult,
	applyKnownModelDefaults,
} from "./applyKnownModelDefaults";
import {
	findKnownModelByCanonicalId,
	type KnownModel,
	type KnownModelSourceMetadata,
} from "./index";

const requireKnownModel = (
	provider: string,
	modelIdentifier: string,
): KnownModel => {
	const knownModel = findKnownModelByCanonicalId(provider, modelIdentifier);
	if (knownModel === undefined) {
		throw new Error(`missing test Known Model: ${provider}/${modelIdentifier}`);
	}
	return knownModel;
};

const getPath = (value: unknown, path: string): unknown => {
	let current = value;
	for (const segment of path.split(".")) {
		if (
			current === null ||
			current === undefined ||
			typeof current !== "object"
		) {
			return undefined;
		}
		current = (current as Record<string, unknown>)[segment];
	}
	return current;
};

const setPath = <T>(value: T, path: string, nextValue: unknown): T => {
	const clone = structuredClone(value);
	let current = clone as Record<string, unknown>;
	const segments = path.split(".");
	for (const segment of segments.slice(0, -1)) {
		const child = current[segment];
		if (child === null || child === undefined || typeof child !== "object") {
			current[segment] = {};
		}
		current = current[segment] as Record<string, unknown>;
	}

	const leaf = segments.at(-1);
	if (leaf === undefined) {
		throw new Error("test path must not be empty");
	}
	current[leaf] = nextValue;
	return clone;
};

const applyDefaults = (
	parameters: ApplyKnownModelDefaultsParameters,
): ApplyKnownModelDefaultsResult => applyKnownModelDefaults(parameters);

const testSourceMetadata = (): KnownModelSourceMetadata => ({
	sourceName: "models.dev",
	sourceRetrievedAt: "2026-04-30",
	lastUpdated: "2026-04-30",
});

const customKnownModel = (overrides: Partial<KnownModel>): KnownModel => ({
	provider: "openai",
	modelIdentifier: "test-model",
	displayName: "Test Model",
	aliases: [],
	sourceMetadata: testSourceMetadata(),
	...overrides,
});

describe("applyKnownModelDefaults", () => {
	it("returns unchanged values and no applied fields for mismatched provider", () => {
		const values = buildInitialModelFormValues();
		const initialValues = buildInitialModelFormValues();
		const result = applyDefaults({
			values,
			initialValues,
			provider: "anthropic",
			knownModel: requireKnownModel("openai", "gpt-5.5"),
		});

		expect(result.values).toBe(values);
		expect(result.appliedFields).toEqual([]);
	});

	it("returns unchanged values and no applied fields for empty provider", () => {
		const values = buildInitialModelFormValues();
		const initialValues = buildInitialModelFormValues();
		const result = applyDefaults({
			values,
			initialValues,
			provider: " ",
			knownModel: requireKnownModel("openai", "gpt-5.5"),
		});

		expect(result.values).toBe(values);
		expect(result.appliedFields).toEqual([]);
	});

	it("populates display name with the Known Model display name", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "anthropic",
			knownModel: requireKnownModel("anthropic", "claude-opus-4-8"),
		});

		expect(result.values.displayName).toBe("Claude Opus 4.8");
		expect(result.appliedFields).toContain("displayName");
	});

	it("skips display name when current value differs from initial value", () => {
		const values = setPath(
			buildInitialModelFormValues(),
			"displayName",
			"My Custom Name",
		);
		const result = applyDefaults({
			values,
			initialValues: buildInitialModelFormValues(),
			provider: "anthropic",
			knownModel: requireKnownModel("anthropic", "claude-opus-4-8"),
		});

		expect(result.values.displayName).toBe("My Custom Name");
		expect(result.appliedFields).not.toContain("displayName");
	});

	it("populates context limit when current value still equals initial value", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "openai",
			knownModel: customKnownModel({ contextLimit: 400_000 }),
		});

		expect(result.values.contextLimit).toBe("400000");
		expect(result.appliedFields).toContain("contextLimit");
	});

	it("skips context limit when current value differs from initial value", () => {
		const values = setPath(
			buildInitialModelFormValues(),
			"contextLimit",
			"123",
		);
		const result = applyDefaults({
			values,
			initialValues: buildInitialModelFormValues(),
			provider: "openai",
			knownModel: customKnownModel({ contextLimit: 400_000 }),
		});

		expect(result.values.contextLimit).toBe("123");
		expect(result.appliedFields).not.toContain("contextLimit");
	});

	it("populates OpenAI output tokens in provider-specific field", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "openai",
			knownModel: customKnownModel({ maxOutputTokens: 128_000 }),
		});

		expect(getPath(result.values, "config.openai.maxCompletionTokens")).toBe(
			"128000",
		);
		expect(getPath(result.values, "config.maxOutputTokens")).toBe("");
		expect(result.appliedFields).toContain("config.openai.maxCompletionTokens");
		expect(result.appliedFields).not.toContain("config.maxOutputTokens");
	});

	it("populates Anthropic output tokens in generic field", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "anthropic",
			knownModel: customKnownModel({
				provider: "anthropic",
				maxOutputTokens: 64_000,
			}),
		});

		expect(getPath(result.values, "config.maxOutputTokens")).toBe("64000");
		expect(
			getPath(result.values, "config.anthropic.maxOutputTokens"),
		).toBeUndefined();
		expect(result.appliedFields).toContain("config.maxOutputTokens");
	});

	it("populates flat input and output costs through pricing descriptors", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "openai",
			knownModel: customKnownModel({ inputCost: 5, outputCost: 30 }),
		});

		expect(
			getPath(result.values, "config.cost.inputPricePerMillionTokens"),
		).toBe("5");
		expect(
			getPath(result.values, "config.cost.outputPricePerMillionTokens"),
		).toBe("30");
		expect(pricingFieldNameList.slice(0, 2)).toEqual([
			"cost.input_price_per_million_tokens",
			"cost.output_price_per_million_tokens",
		]);
		expect(result.appliedFields).toEqual(
			expect.arrayContaining([
				"config.cost.inputPricePerMillionTokens",
				"config.cost.outputPricePerMillionTokens",
			]),
		);
	});

	it("populates cache read and cache write costs when present", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "anthropic",
			knownModel: customKnownModel({
				provider: "anthropic",
				cacheReadCost: 0.5,
				cacheWriteCost: 6.25,
			}),
		});

		expect(
			getPath(result.values, "config.cost.cacheReadPricePerMillionTokens"),
		).toBe("0.5");
		expect(
			getPath(result.values, "config.cost.cacheWritePricePerMillionTokens"),
		).toBe("6.25");
		expect(result.appliedFields).toEqual(
			expect.arrayContaining([
				"config.cost.cacheReadPricePerMillionTokens",
				"config.cost.cacheWritePricePerMillionTokens",
			]),
		);
	});

	it("leaves missing cache costs unchanged and excludes them from applied fields", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "openai",
			knownModel: customKnownModel({ inputCost: 30, outputCost: 180 }),
		});

		expect(
			getPath(result.values, "config.cost.cacheReadPricePerMillionTokens"),
		).toBe("");
		expect(
			getPath(result.values, "config.cost.cacheWritePricePerMillionTokens"),
		).toBe("");
		expect(result.appliedFields).not.toContain(
			"config.cost.cacheReadPricePerMillionTokens",
		);
		expect(result.appliedFields).not.toContain(
			"config.cost.cacheWritePricePerMillionTokens",
		);
	});

	it("does not set compressionThreshold", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "openai",
			knownModel: requireKnownModel("openai", "gpt-5.5"),
		});

		expect(result.values.compressionThreshold).toBe("");
		expect(result.appliedFields).not.toContain("compressionThreshold");
	});

	it("does not set reasoning effort fields without catalog defaults", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "openai",
			knownModel: requireKnownModel("openai", "gpt-5.4"),
		});

		expect(getPath(result.values, "config.reasoningEffort.default")).toBe("");
		expect(getPath(result.values, "config.reasoningEffort.max")).toBe("");
		expect(getPath(result.values, "config.openai.reasoningSummary")).toBe("");
		expect(result.appliedFields).not.toContain(
			"config.reasoningEffort.default",
		);
		expect(result.appliedFields).not.toContain("config.reasoningEffort.max");
		expect(result.appliedFields).not.toContain(
			"config.openai.reasoningSummary",
		);
	});

	it("sets reasoning effort bounds for reasoning-capable catalog entries", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "openai",
			knownModel: requireKnownModel("openai", "gpt-5.5"),
		});

		expect(getPath(result.values, "config.reasoningEffort.default")).toBe(
			"medium",
		);
		expect(getPath(result.values, "config.reasoningEffort.max")).toBe("medium");
		expect(getPath(result.values, "config.openai.reasoningSummary")).toBe("");
		expect(result.appliedFields).toContain("config.reasoningEffort.default");
		expect(result.appliedFields).toContain("config.reasoningEffort.max");
		expect(result.appliedFields).not.toContain(
			"config.openai.reasoningSummary",
		);
	});

	it("sets reasoning effort bounds for Anthropic extended-thinking catalog entries", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "anthropic",
			knownModel: requireKnownModel("anthropic", "claude-opus-4-8"),
		});

		expect(getPath(result.values, "config.reasoningEffort.default")).toBe(
			"high",
		);
		expect(getPath(result.values, "config.reasoningEffort.max")).toBe("high");
		expect(result.appliedFields).toContain("config.reasoningEffort.default");
		expect(result.appliedFields).toContain("config.reasoningEffort.max");
	});

	it("sets reasoning effort for any provider with a catalog default", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "google",
			knownModel: customKnownModel({
				provider: "google",
				reasoningEffort: "medium",
			}),
		});

		expect(getPath(result.values, "config.reasoningEffort.default")).toBe(
			"medium",
		);
		expect(getPath(result.values, "config.reasoningEffort.max")).toBe("medium");
		expect(result.appliedFields).toContain("config.reasoningEffort.default");
		expect(result.appliedFields).toContain("config.reasoningEffort.max");
	});

	it.each([
		"claude-haiku-4-5",
		"claude-sonnet-4-5",
	])("sets Anthropic thinking budget instead of effort for %s", (modelIdentifier) => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "anthropic",
			knownModel: requireKnownModel("anthropic", modelIdentifier),
		});

		expect(
			getPath(result.values, "config.anthropic.thinking.budgetTokens"),
		).toBe("8192");
		expect(result.appliedFields).toContain(
			"config.anthropic.thinking.budgetTokens",
		);
		expect(getPath(result.values, "config.reasoningEffort.default")).toBe("");
		expect(getPath(result.values, "config.reasoningEffort.max")).toBe("");
		expect(result.appliedFields).not.toContain(
			"config.reasoningEffort.default",
		);
		expect(result.appliedFields).not.toContain("config.reasoningEffort.max");
	});

	it("does not set Anthropic sendReasoning or thinking budget fields", () => {
		const result = applyDefaults({
			values: buildInitialModelFormValues(),
			initialValues: buildInitialModelFormValues(),
			provider: "anthropic",
			knownModel: requireKnownModel("anthropic", "claude-opus-4-8"),
		});

		expect(getPath(result.values, "config.anthropic.sendReasoning")).toBe("");
		expect(
			getPath(result.values, "config.anthropic.thinking.budgetTokens"),
		).toBe("");
		expect(result.appliedFields).not.toContain(
			"config.anthropic.sendReasoning",
		);
		expect(result.appliedFields).not.toContain(
			"config.anthropic.thinking.budgetTokens",
		);
	});

	it("never includes model in applied fields", () => {
		const values = setPath(
			buildInitialModelFormValues(),
			"model",
			"typed-model",
		);
		const result = applyDefaults({
			values,
			initialValues: buildInitialModelFormValues(),
			provider: "openai",
			knownModel: requireKnownModel("openai", "gpt-5.5"),
		});

		const { model: resultModel } = result.values;
		expect(resultModel).toBe("typed-model");
		expect(result.appliedFields).not.toContain("model");
	});

	it("does not mutate original values or initialValues", () => {
		const values = buildInitialModelFormValues();
		const initialValues = buildInitialModelFormValues();
		const knownModel = requireKnownModel("openai", "gpt-5.5");
		const valuesBefore = structuredClone(values);
		const initialValuesBefore = structuredClone(initialValues);
		const knownModelBefore = structuredClone(knownModel);

		const result = applyDefaults({
			values,
			initialValues,
			provider: "openai",
			knownModel,
		});

		expect(result.values).not.toBe(values);
		expect(values).toEqual(valuesBefore);
		expect(initialValues).toEqual(initialValuesBefore);
		expect(knownModel).toEqual(knownModelBefore);
	});
});
