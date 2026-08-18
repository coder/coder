import type { AIModelPrice, AIModelPriceUpsert } from "#/api/typesGenerated";

const microsPerDollar = 1_000_000;
const microsPerDollarBigInt = BigInt(microsPerDollar);

export type ModelPricingFormValues = {
	inputPrice: string;
	outputPrice: string;
	cacheReadPrice: string;
	cacheWritePrice: string;
};

export type ModelPricingFormErrors = Partial<
	Record<keyof ModelPricingFormValues, string>
>;

const formatDollarPrice = (micros: number | null): string => {
	if (micros === null || micros < 0 || !Number.isSafeInteger(micros)) {
		return "";
	}
	const microsBigInt = BigInt(micros);
	const wholeDollars = microsBigInt / microsPerDollarBigInt;
	const fractionalMicros = microsBigInt % microsPerDollarBigInt;
	if (fractionalMicros === 0n) {
		return wholeDollars.toString();
	}
	const fraction = fractionalMicros
		.toString()
		.padStart(6, "0")
		.replace(/0+$/, "");
	return `${wholeDollars}.${fraction}`;
};

export const modelPricingFormValues = (
	price: AIModelPrice | undefined,
): ModelPricingFormValues => ({
	inputPrice: formatDollarPrice(price?.input_price ?? null),
	outputPrice: formatDollarPrice(price?.output_price ?? null),
	cacheReadPrice: formatDollarPrice(price?.cache_read_price ?? null),
	cacheWritePrice: formatDollarPrice(price?.cache_write_price ?? null),
});

const parseDollarPrice = (value: string): number | null | undefined => {
	const trimmed = value.trim();
	if (trimmed === "") {
		return null;
	}
	const match = /^(\d+)(?:\.(\d{1,6}))?$/.exec(trimmed);
	if (!match) {
		return undefined;
	}
	const wholeMicros = BigInt(match[1]) * microsPerDollarBigInt;
	const fractionalMicros = BigInt((match[2] ?? "").padEnd(6, "0"));
	const micros = wholeMicros + fractionalMicros;
	if (micros > BigInt(Number.MAX_SAFE_INTEGER)) {
		return undefined;
	}
	return Number(micros);
};

export const validateModelPricing = (
	values: ModelPricingFormValues,
): ModelPricingFormErrors => {
	const errors: ModelPricingFormErrors = {};
	const fields: readonly (keyof ModelPricingFormValues)[] = [
		"inputPrice",
		"outputPrice",
		"cacheReadPrice",
		"cacheWritePrice",
	];
	for (const field of fields) {
		if (parseDollarPrice(values[field]) === undefined) {
			errors[field] =
				"Enter a non-negative USD amount with up to 6 decimal places, or leave it blank if unknown.";
		}
	}
	return errors;
};

export const buildModelPriceUpsert = (
	provider: string,
	model: string,
	values: ModelPricingFormValues,
): AIModelPriceUpsert | null => {
	const parsed = {
		input_price: parseDollarPrice(values.inputPrice),
		output_price: parseDollarPrice(values.outputPrice),
		cache_read_price: parseDollarPrice(values.cacheReadPrice),
		cache_write_price: parseDollarPrice(values.cacheWritePrice),
	};
	if (Object.values(parsed).some((value) => value === undefined)) {
		return null;
	}
	if (Object.values(parsed).every((value) => value === null)) {
		return null;
	}
	return {
		provider,
		model,
		input_price: parsed.input_price ?? null,
		output_price: parsed.output_price ?? null,
		cache_read_price: parsed.cache_read_price ?? null,
		cache_write_price: parsed.cache_write_price ?? null,
	};
};
