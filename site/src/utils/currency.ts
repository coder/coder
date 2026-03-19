export const MICROS_PER_DOLLAR = 1_000_000;

const usdCurrencyFormatter = new Intl.NumberFormat("en-US", {
	style: "currency",
	currency: "USD",
	minimumFractionDigits: 2,
	maximumFractionDigits: 2,
	signDisplay: "auto",
});

const usdSubCentCurrencyFormatter = new Intl.NumberFormat("en-US", {
	style: "currency",
	currency: "USD",
	minimumFractionDigits: 4,
	maximumFractionDigits: 4,
	signDisplay: "auto",
});

export function microsToDollars(micros: number): number {
	return micros / MICROS_PER_DOLLAR;
}

export function dollarsToMicros(dollars: string | number): number {
	if (typeof dollars === "string" && dollars.trim() === "") {
		return 0;
	}

	const micros = Math.round(Number(dollars) * MICROS_PER_DOLLAR);
	return Number.isFinite(micros) && micros > 0 ? micros : 0;
}

export function isPositiveFiniteDollarAmount(dollars: string): boolean {
	return dollars.trim() !== "" && dollarsToMicros(dollars) > 0;
}

export function formatCostMicros(micros: number | string): string {
	const microsValue = Number(micros);
	if (!Number.isFinite(microsValue)) {
		return "$0.00";
	}

	const dollars = Math.abs(microsValue) / MICROS_PER_DOLLAR;
	const rounded4 = Number(dollars.toFixed(4));
	if (rounded4 > 0 && rounded4 < 0.01) {
		if (microsValue < 0) {
			return `-$${dollars.toFixed(4)}`;
		}

		return usdSubCentCurrencyFormatter.format(dollars);
	}

	return usdCurrencyFormatter.format(microsToDollars(microsValue));
}
