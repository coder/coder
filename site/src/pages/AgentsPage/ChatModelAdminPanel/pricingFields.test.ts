import type * as TypesGen from "api/typesGenerated";
import { describe, expect, it } from "vitest";
import {
	getDefaultPricingForField,
	getPricingPlaceholderForField,
	hasCustomPricing,
	pricingFieldNameList,
} from "./pricingFields";

describe("pricingFields", () => {
	it("uses $0 defaults for every pricing field", () => {
		for (const fieldName of pricingFieldNameList) {
			expect(getDefaultPricingForField(fieldName)).toBe(0);
			expect(getPricingPlaceholderForField(fieldName)).toBe("0");
		}
	});

	it("treats missing pricing as undefined pricing", () => {
		expect(hasCustomPricing()).toBe(false);
	});

	it("treats explicit zero pricing as custom pricing", () => {
		expect(
			hasCustomPricing({
				cost: {
					input_price_per_million_tokens: 0,
					output_price_per_million_tokens: 0,
				},
			} satisfies TypesGen.ChatModelCallConfig),
		).toBe(true);
	});

	it("detects custom pricing when any pricing field is greater than zero", () => {
		expect(
			hasCustomPricing({
				cost: {
					cache_write_price_per_million_tokens: 0.25,
				},
			} satisfies TypesGen.ChatModelCallConfig),
		).toBe(true);
	});
});
