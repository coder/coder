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
 * Prices the tier leaves unset fall back to the base price, matching how the
 * server bills them, except an unset 1h price, which uses the cache write
 * price that applies to the tier.
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
	return `Over ${tier.threshold_tokens.toLocaleString("en-US")} prompt tokens (USD/1M tokens): ${prices.join(" · ")}`;
};

type PriceTierNotesProps = {
	price: AIModelPrice;
};

/** The server keeps tiers in ascending threshold order. */
export const PriceTierNotes: React.FC<PriceTierNotesProps> = ({ price }) =>
	price.tiers.map((tier) => (
		<p
			key={tier.threshold_tokens}
			className="col-span-full m-0 text-xs text-content-secondary"
		>
			{formatTierNote(price, tier)}
		</p>
	));
