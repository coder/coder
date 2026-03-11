import type * as TypesGen from "api/typesGenerated";

// Single source of truth for the top-level model config fields that belong
// in the Pricing section and require non-negative validation.
export const pricingFieldNameList = [
	"input_price_per_million_tokens",
	"output_price_per_million_tokens",
	"cache_read_price_per_million_tokens",
	"cache_write_price_per_million_tokens",
] as const;

export const pricingFieldNames = new Set<string>(pricingFieldNameList);

type PricingFieldName = (typeof pricingFieldNameList)[number];

export const defaultPricingByFieldName = {
	input_price_per_million_tokens: 0,
	output_price_per_million_tokens: 0,
	cache_read_price_per_million_tokens: 0,
	cache_write_price_per_million_tokens: 0,
} as const satisfies Record<PricingFieldName, number>;

export const pricingPlaceholderByFieldName = {
	input_price_per_million_tokens: "0",
	output_price_per_million_tokens: "0",
	cache_read_price_per_million_tokens: "0",
	cache_write_price_per_million_tokens: "0",
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

export const hasCustomPricing = (
	modelConfig?: TypesGen.ChatModelCallConfig,
): boolean =>
	pricingFieldNameList.some(
		(fieldName) => modelConfig?.[fieldName] !== undefined,
	);
