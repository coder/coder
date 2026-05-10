import { toFormFieldKey } from "#/api/chatModelOptions";
import {
	deepGet,
	deepSet,
	type ModelFormValues,
} from "../modelConfigFormLogic";
import { pricingFieldNameList } from "../pricingFields";
import type { KnownModel } from "./types";

export type ApplyKnownModelDefaultsResult = {
	values: ModelFormValues;
	appliedFields: readonly string[];
};

export type ApplyKnownModelDefaultsParameters = {
	values: ModelFormValues;
	initialValues: ModelFormValues;
	provider: string;
	knownModel: KnownModel;
};

type KnownModelCostField =
	| "inputCost"
	| "outputCost"
	| "cacheReadCost"
	| "cacheWriteCost";

const pricingModelFieldByName = {
	"cost.input_price_per_million_tokens": "inputCost",
	"cost.output_price_per_million_tokens": "outputCost",
	"cost.cache_read_price_per_million_tokens": "cacheReadCost",
	"cost.cache_write_price_per_million_tokens": "cacheWriteCost",
} as const satisfies Record<
	(typeof pricingFieldNameList)[number],
	KnownModelCostField
>;

const reasoningEffortPathByProvider: Record<string, string> = {
	openai: "config.openai.reasoningEffort",
	anthropic: "config.anthropic.effort",
};

const thinkingBudgetTokensPathByProvider: Record<string, string> = {
	anthropic: "config.anthropic.thinking.budgetTokens",
};

const maybeApplyDefault = ({
	appliedFields,
	initialValues,
	nextValues,
	path,
	value,
	values,
}: {
	appliedFields: string[];
	initialValues: ModelFormValues;
	nextValues: ModelFormValues;
	path: string;
	value: string;
	values: ModelFormValues;
}): void => {
	const segments = path.split(".");
	if (deepGet(values, segments) !== deepGet(initialValues, segments)) {
		return;
	}

	deepSet(nextValues as Record<string, unknown>, segments, value);
	appliedFields.push(path);
};

// Writes Known Model defaults only to fields still at their initial value;
// never overrides user edits. Pure helper independent of Formik touched state.
export const applyKnownModelDefaults = ({
	values,
	initialValues,
	provider,
	knownModel,
}: ApplyKnownModelDefaultsParameters): ApplyKnownModelDefaultsResult => {
	if (provider.trim() === "" || knownModel.provider !== provider) {
		return { values, appliedFields: [] };
	}

	const nextValues = structuredClone(values);
	const appliedFields: string[] = [];

	if (knownModel.contextLimit !== undefined) {
		maybeApplyDefault({
			appliedFields,
			initialValues,
			nextValues,
			path: "contextLimit",
			value: String(knownModel.contextLimit),
			values,
		});
	}

	if (knownModel.maxOutputTokens !== undefined) {
		maybeApplyDefault({
			appliedFields,
			initialValues,
			nextValues,
			path:
				provider === "openai"
					? "config.openai.maxCompletionTokens"
					: "config.maxOutputTokens",
			value: String(knownModel.maxOutputTokens),
			values,
		});
	}

	if (knownModel.reasoningEffort !== undefined) {
		// The catalog uses a single `reasoningEffort` field, but each provider
		// exposes it under a different form path: OpenAI as `reasoningEffort`,
		// Anthropic as `effort`. Providers without a mapping skip this default.
		const reasoningEffortPath = reasoningEffortPathByProvider[provider];
		if (reasoningEffortPath !== undefined) {
			maybeApplyDefault({
				appliedFields,
				initialValues,
				nextValues,
				path: reasoningEffortPath,
				value: knownModel.reasoningEffort,
				values,
			});
		}
	}

	if (knownModel.thinkingBudgetTokens !== undefined) {
		const path = thinkingBudgetTokensPathByProvider[provider];
		if (path !== undefined) {
			maybeApplyDefault({
				appliedFields,
				initialValues,
				nextValues,
				path,
				value: String(knownModel.thinkingBudgetTokens),
				values,
			});
		}
	}

	for (const fieldName of pricingFieldNameList) {
		const knownModelField = pricingModelFieldByName[fieldName];
		const cost = knownModel[knownModelField];
		if (cost === undefined) {
			continue;
		}
		const path = toFormFieldKey("config", fieldName);
		maybeApplyDefault({
			appliedFields,
			initialValues,
			nextValues,
			path,
			value: String(cost),
			values,
		});
	}

	return { values: nextValues, appliedFields };
};
