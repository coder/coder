import type { AIModelPrice, AIModelPriceTier } from "#/api/typesGenerated";
import { MockClaudeSonnetModelPrice } from "#/testHelpers/chatModels";
import { formatTierNote } from "./PriceTierNotes";

describe("formatTierNote", () => {
	const [firstTier, secondTier] = MockClaudeSonnetModelPrice.tiers;

	it("lists every tier price, including the 1h price", () => {
		expect(formatTierNote(MockClaudeSonnetModelPrice, firstTier)).toBe(
			"Over 200,000 prompt tokens (USD/1M tokens): input $6 · output $22.50 · cache read $0.60 · cache write $7.50 · cache write (1h) $12",
		);
	});

	it("uses the base price for an unset tier price and the tier cache write price for an unset 1h price", () => {
		expect(formatTierNote(MockClaudeSonnetModelPrice, secondTier)).toBe(
			"Over 500,000 prompt tokens (USD/1M tokens): input $9 · output $30 · cache read $0.30 · cache write $10 · cache write (1h) $10",
		);
	});

	it("uses the base prices for a tier that sets none, and the cache write price for 1h", () => {
		const mockUnpricedTier: AIModelPriceTier = {
			threshold_tokens: 300000,
			input_price: null,
			output_price: null,
			cache_read_price: null,
			cache_write_price: null,
			cache_write_1h_price: null,
		};
		expect(formatTierNote(MockClaudeSonnetModelPrice, mockUnpricedTier)).toBe(
			"Over 300,000 prompt tokens (USD/1M tokens): input $3 · output $15 · cache read $0.30 · cache write $3.75 · cache write (1h) $3.75",
		);
	});

	it("lists a tier 1h price when the base has none", () => {
		const mockPriceWithout1h: AIModelPrice = {
			...MockClaudeSonnetModelPrice,
			cache_write_1h_price: null,
		};
		expect(formatTierNote(mockPriceWithout1h, firstTier)).toBe(
			"Over 200,000 prompt tokens (USD/1M tokens): input $6 · output $22.50 · cache read $0.60 · cache write $7.50 · cache write (1h) $12",
		);
	});

	it("marks unset and tiny prices and omits an unset 1h price", () => {
		const mockPriceWithoutCacheReadOr1h: AIModelPrice = {
			...MockClaudeSonnetModelPrice,
			cache_read_price: null,
			cache_write_1h_price: null,
		};
		const mockTinyInputTier: AIModelPriceTier = {
			...secondTier,
			input_price: 1,
		};
		expect(
			formatTierNote(mockPriceWithoutCacheReadOr1h, mockTinyInputTier),
		).toBe(
			"Over 500,000 prompt tokens (USD/1M tokens): input <$0.0001 · output $30 · cache read N/A · cache write $10",
		);
	});
});
