// Single source of truth for the top-level model config fields that belong
// in the Pricing section and require non-negative validation.
export const pricingFieldNames = new Set<string>([
	"input_price_per_million_tokens",
	"output_price_per_million_tokens",
	"cache_read_price_per_million_tokens",
	"cache_write_price_per_million_tokens",
]);

export const defaultPricingByFieldName = {
	input_price_per_million_tokens: 5,
	output_price_per_million_tokens: 20,
	cache_read_price_per_million_tokens: 0.5,
	cache_write_price_per_million_tokens: 5,
} as const satisfies Partial<Record<string, number>>;

export const pricingPlaceholderByFieldName = {
	input_price_per_million_tokens: "5",
	output_price_per_million_tokens: "20",
	cache_read_price_per_million_tokens: "0.50",
	cache_write_price_per_million_tokens: "5",
} as const satisfies Partial<Record<string, string>>;

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
