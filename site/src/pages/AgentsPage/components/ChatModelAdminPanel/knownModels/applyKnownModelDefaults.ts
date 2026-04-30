import { snakeToCamel, toFormFieldKey } from "#/api/chatModelOptions";
import {
	deepGet,
	deepSet,
	type ModelFormValues,
} from "../modelConfigFormLogic";
import { pricingFieldNameList } from "../pricingFields";
import type { KnownModel } from "./types";

// This helper preserves any field that has drifted from the form's initial
// value. It writes advisory Known Model defaults only when the current leaf
// value still strictly equals that same leaf path in initialValues, because
// this pure helper deliberately does not depend on Formik touched state.
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

	for (const fieldName of pricingFieldNameList) {
		const knownModelField = pricingModelFieldByName[fieldName];
		const cost = knownModel[knownModelField];
		if (cost === undefined) {
			continue;
		}
		const path = toFormFieldKey("config", fieldName);
		const previousPath = `config.${fieldName
			.split(".")
			.map(snakeToCamel)
			.join(".")}`;
		if (path !== previousPath) {
			throw new Error("pricing field form path must match legacy path");
		}
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
