import {
	composeFilterQuery,
	extractFreeText,
	parseChipToken,
	queryToChips,
} from "#/components/Filter/FilterCombobox/filterQuery";
import type { SpendDimensions } from "./SpendFilters";

// Query keys shared by the spend API and the AI Sessions filter. Keeping them
// identical lets the drill-in hand its dimensions to the sessions link as-is.
const SPEND_CHIP_KEYS = [
	"provider_name",
	"client",
	"model",
] as const satisfies readonly (keyof SpendDimensions)[];

// Serializes the discrete spend params (the canonical URL state) into the single
// query string the FilterCombobox consumes: one chip per dimension plus the user
// search as trailing free text.
export const spendFilterToQuery = (
	dimensions: SpendDimensions,
	search: string,
): string => {
	const tokens: string[] = [];
	for (const key of SPEND_CHIP_KEYS) {
		const value = dimensions[key];
		if (value) {
			tokens.push(`${key}:${value}`);
		}
	}
	return composeFilterQuery(tokens, SPEND_CHIP_KEYS, search);
};

// Round-trip partner of spendFilterToQuery: parses a combobox query back into
// the discrete dimensions and the free-text user search.
export const queryToSpendFilter = (
	query: string,
): { dimensions: SpendDimensions; search: string } => {
	const dimensions: Record<string, string> = {};
	for (const token of queryToChips(query, SPEND_CHIP_KEYS)) {
		const parsed = parseChipToken(token, SPEND_CHIP_KEYS);
		if (parsed) {
			dimensions[parsed.key] = parsed.value;
		}
	}
	return { dimensions, search: extractFreeText(query, SPEND_CHIP_KEYS) };
};
