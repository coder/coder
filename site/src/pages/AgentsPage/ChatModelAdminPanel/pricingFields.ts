import type { FieldSchema } from "api/chatModelOptions";

const pricingFieldNames = new Set<string>([
	"input_price_per_million_tokens",
	"output_price_per_million_tokens",
	"cache_read_price_per_million_tokens",
	"cache_write_price_per_million_tokens",
]);

export const isPricingField = (
	field: Pick<FieldSchema, "json_name">,
): boolean => pricingFieldNames.has(field.json_name);
