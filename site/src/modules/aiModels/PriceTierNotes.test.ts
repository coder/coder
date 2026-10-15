import type { AIModelPrice } from "#/api/typesGenerated";
import { MockClaudeSonnetModelPrice } from "#/testHelpers/chatModels";
import { formatTierNote } from "./PriceTierNotes";

describe("formatTierNote", () => {
	const [firstTier, secondTier] = MockClaudeSonnetModelPrice.tiers;

	it("lists every tier price, including the 1h price", () => {
		expect(formatTierNote(MockClaudeSonnetModelPrice, firstTier)).toBe(
			"Over 200,000 prompt tokens: input $6 · output $22.50 · cache read $0.60 · cache write $7.50 · cache write (1h) $12",
		);
	});

	it("uses the base price for an unset tier price and omits an unset 1h price", () => {
		expect(formatTierNote(MockClaudeSonnetModelPrice, secondTier)).toBe(
			"Over 500,000 prompt tokens: input $9 · output $30 · cache read $0.30 · cache write $10",
		);
	});

	it("marks prices unset on both the tier and the base, and tiny prices", () => {
		const base: AIModelPrice = {
			...MockClaudeSonnetModelPrice,
			cache_read_price: null,
		};
		expect(
			formatTierNote(base, {
				...secondTier,
				input_price: 1,
			}),
		).toBe(
			"Over 500,000 prompt tokens: input <$0.0001 · output $30 · cache read N/A · cache write $10",
		);
	});
});
