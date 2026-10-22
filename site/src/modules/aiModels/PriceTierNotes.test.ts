import type { AIModelPrice, AIModelPriceTier } from "#/api/typesGenerated";
import { MockClaudeSonnetModelPrice } from "#/testHelpers/chatModels";
import { formatTierNote } from "./PriceTierNotes";

describe("formatTierNote", () => {
	const [firstTier, secondTier] = MockClaudeSonnetModelPrice.tiers;

	it("lists every tier price", () => {
		expect(formatTierNote(MockClaudeSonnetModelPrice, firstTier)).toBe(
			"Over 200,000 prompt tokens (USD/1M tokens): input $6 · output $22.50 · cache read $0.60 · cache write $7.50",
		);
	});

	it("uses the base price for an unset tier price", () => {
		expect(formatTierNote(MockClaudeSonnetModelPrice, secondTier)).toBe(
			"Over 500,000 prompt tokens (USD/1M tokens): input $9 · output $30 · cache read $0.30 · cache write $10",
		);
	});

	it("uses every base price for a tier that sets none", () => {
		const mockUnpricedTier: AIModelPriceTier = {
			threshold_tokens: 300000,
			input_price: null,
			output_price: null,
			cache_read_price: null,
			cache_write_price: null,
		};
		expect(formatTierNote(MockClaudeSonnetModelPrice, mockUnpricedTier)).toBe(
			"Over 300,000 prompt tokens (USD/1M tokens): input $3 · output $15 · cache read $0.30 · cache write $3.75",
		);
	});

	it("marks prices unset on both the tier and the base, and tiny prices", () => {
		const mockPriceWithoutCacheRead: AIModelPrice = {
			...MockClaudeSonnetModelPrice,
			cache_read_price: null,
		};
		const mockTinyInputTier: AIModelPriceTier = {
			...secondTier,
			input_price: 1,
		};
		expect(formatTierNote(mockPriceWithoutCacheRead, mockTinyInputTier)).toBe(
			"Over 500,000 prompt tokens (USD/1M tokens): input <$0.0001 · output $30 · cache read N/A · cache write $10",
		);
	});
});
