// Single source of truth for the top-level model config fields that belong
// in the Pricing section and require non-negative validation.
export const pricingFieldNames = new Set<string>([
	"input_price_per_million_tokens",
	"output_price_per_million_tokens",
	"cache_read_price_per_million_tokens",
	"cache_write_price_per_million_tokens",
]);
