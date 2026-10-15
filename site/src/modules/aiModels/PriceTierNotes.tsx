import type { AIModelPrice, AIModelPriceTier } from "#/api/typesGenerated";
import { microsToDollars } from "#/utils/currency";
import { formatPricePerMillionTokens } from "./knownModels/catalog";

const formatLabeledPrice = (label: string, micros: number | null): string => {
	if (micros === null) {
		return `${label} N/A`;
	}
	const price = formatPricePerMillionTokens(microsToDollars(micros));
	return `${label} ${price.belowThreshold ? "<$" : "$"}${price.value}`;
};

/**
 * Formats the note for one long-context tier of a price row. A tier without a
 * 1h price bills 1h writes at its own cache write price, not at the base 1h
 * price.
 */
export const formatTierNote = (
	base: AIModelPrice,
	tier: AIModelPriceTier,
): string => {
	const cacheWrite = tier.cache_write_price ?? base.cache_write_price;
	const prices = [
		formatLabeledPrice("input", tier.input_price ?? base.input_price),
		formatLabeledPrice("output", tier.output_price ?? base.output_price),
		formatLabeledPrice(
			"cache read",
			tier.cache_read_price ?? base.cache_read_price,
		),
		formatLabeledPrice("cache write", cacheWrite),
	];
	if (
		base.cache_write_1h_price !== null ||
		tier.cache_write_1h_price !== null
	) {
		prices.push(
			formatLabeledPrice(
				"cache write (1h)",
				tier.cache_write_1h_price ?? cacheWrite,
			),
		);
	}
	return `Over ${tier.threshold_tokens.toLocaleString("en-US")} prompt tokens: ${prices.join(" · ")}`;
};

/** One note per long-context price tier of a price row. */
export const PriceTierNotes: React.FC<{ price: AIModelPrice }> = ({
	price,
}) => (
	<>
		{price.tiers.map((tier) => (
			<p
				key={tier.threshold_tokens}
				className="col-span-full m-0 text-xs text-content-secondary"
			>
				{formatTierNote(price, tier)}
			</p>
		))}
	</>
);
