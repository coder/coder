import type { ModelFormValues } from "../modelConfigFormLogic";
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

const snakeToCamel = (value: string): string =>
	value.replace(/_([a-z0-9])/g, (_, character: string) =>
		character.toUpperCase(),
	);

const formPathForPricingField = (fieldName: string): string =>
	`config.${fieldName.split(".").map(snakeToCamel).join(".")}`;

const getPath = (value: unknown, path: readonly string[]): unknown => {
	let current = value;
	for (const segment of path) {
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

const setPath = (
	value: unknown,
	path: readonly string[],
	nextValue: string,
): void => {
	if (value === null || value === undefined || typeof value !== "object") {
		throw new Error("default application target must be an object");
	}

	let current = value as Record<string, unknown>;
	for (const segment of path.slice(0, -1)) {
		const child = current[segment];
		if (child === null || child === undefined || typeof child !== "object") {
			current[segment] = {};
		}
		current = current[segment] as Record<string, unknown>;
	}

	const leaf = path.at(-1);
	if (leaf === undefined) {
		throw new Error("default application path must not be empty");
	}
	current[leaf] = nextValue;
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
	if (getPath(values, segments) !== getPath(initialValues, segments)) {
		return;
	}

	setPath(nextValues, segments, value);
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
		maybeApplyDefault({
			appliedFields,
			initialValues,
			nextValues,
			path: formPathForPricingField(fieldName),
			value: String(cost),
			values,
		});
	}

	return { values: nextValues, appliedFields };
};
