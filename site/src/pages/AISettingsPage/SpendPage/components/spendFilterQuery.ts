import type { OrganizationAISpendFilter } from "#/api/typesGenerated";
import {
	composeFilterQuery,
	extractFreeText,
	parseChipToken,
	queryToChips,
} from "#/components/Filter/FilterCombobox/filterQuery";
export type SpendDimensions = Pick<
	OrganizationAISpendFilter,
	"provider_name" | "client" | "model"
>;

const SPEND_CHIP_KEYS = [
	"provider_name",
	"client",
	"model",
] as const satisfies readonly (keyof SpendDimensions)[];

/** Combines URL dimensions with local free text for the combobox. */
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

/** Separates supported API dimensions from free text for validation. */
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
