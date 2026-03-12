import type * as TypesGen from "api/typesGenerated";

// Single source of truth for the model config fields that belong in the
// Pricing section and require non-negative validation.
export const pricingFieldNameList = [
	"cost.input_price_per_million_tokens",
	"cost.output_price_per_million_tokens",
	"cost.cache_read_price_per_million_tokens",
	"cost.cache_write_price_per_million_tokens",
] as const;

export const pricingFieldNames = new Set<string>(pricingFieldNameList);

type PricingFieldName = (typeof pricingFieldNameList)[number];

export const defaultPricingByFieldName = {
	"cost.input_price_per_million_tokens": 0,
	"cost.output_price_per_million_tokens": 0,
	"cost.cache_read_price_per_million_tokens": 0,
	"cost.cache_write_price_per_million_tokens": 0,
} as const satisfies Record<PricingFieldName, number>;

export const pricingPlaceholderByFieldName = {
	"cost.input_price_per_million_tokens": "0",
	"cost.output_price_per_million_tokens": "0",
	"cost.cache_read_price_per_million_tokens": "0",
	"cost.cache_write_price_per_million_tokens": "0",
} as const satisfies Record<PricingFieldName, string>;

export const getDefaultPricingForField = (
	fieldName: string,
): number | undefined =>
	defaultPricingByFieldName[
		fieldName as keyof typeof defaultPricingByFieldName
	];

export const getPricingPlaceholderForField = (
	fieldName: string,
): string | undefined =>
	pricingPlaceholderByFieldName[
		fieldName as keyof typeof pricingPlaceholderByFieldName
	];

const getNestedValue = (value: unknown, path: readonly string[]): unknown => {
	let current = value;
	for (const segment of path) {
		if (
			current === undefined ||
			current === null ||
			typeof current !== "object"
		) {
			return undefined;
		}
		current = (current as Record<string, unknown>)[segment];
	}
	return current;
};

export const hasCustomPricing = (
	modelConfig?: TypesGen.ChatModelCallConfig,
): boolean =>
	pricingFieldNameList.some(
		(fieldName) =>
			getNestedValue(modelConfig, fieldName.split(".")) !== undefined,
	);
