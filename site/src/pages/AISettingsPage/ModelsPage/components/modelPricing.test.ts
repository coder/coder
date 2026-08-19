import { describe, expect, it } from "vitest";
import {
	buildModelPriceUpsert,
	modelPricingFormValues,
	validateModelPricing,
} from "./modelPricing";

const blankPrices = {
	inputPrice: "",
	outputPrice: "",
	cacheReadPrice: "",
	cacheWritePrice: "",
};

describe("model pricing", () => {
	it("converts decimal prices to micro-units without floating-point rounding", () => {
		expect(
			buildModelPriceUpsert("openai", "custom-model", {
				...blankPrices,
				inputPrice: "0.0079",
				outputPrice: "1.234567",
			}),
		).toEqual({
			provider: "openai",
			model: "custom-model",
			input_price: 7_900,
			output_price: 1_234_567,
			cache_read_price: null,
			cache_write_price: null,
		});
	});

	it("round-trips prices near the maximum safe integer exactly", () => {
		const price = {
			provider: "openai",
			model: "custom-model",
			input_price: Number.MAX_SAFE_INTEGER,
			output_price: Number.MAX_SAFE_INTEGER - 1,
			cache_read_price: 1,
			cache_write_price: 1_000_001,
			default: false,
			created_at: "2026-08-18T12:00:00.000Z",
			updated_at: "2026-08-18T12:00:00.000Z",
		};

		const values = modelPricingFormValues(price);
		expect(values).toEqual({
			inputPrice: "9007199254.740991",
			outputPrice: "9007199254.74099",
			cacheReadPrice: "0.000001",
			cacheWritePrice: "1.000001",
		});
		expect(buildModelPriceUpsert(price.provider, price.model, values)).toEqual({
			provider: price.provider,
			model: price.model,
			input_price: price.input_price,
			output_price: price.output_price,
			cache_read_price: price.cache_read_price,
			cache_write_price: price.cache_write_price,
		});
	});

	it("rejects prices that cannot be represented as safe integer micro-units", () => {
		const values = {
			...blankPrices,
			inputPrice: "9007199254.740992",
		};

		expect(validateModelPricing(values)).toHaveProperty("inputPrice");
		expect(buildModelPriceUpsert("openai", "custom-model", values)).toBeNull();
	});
});
