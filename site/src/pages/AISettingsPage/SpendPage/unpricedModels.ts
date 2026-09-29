import type {
	AIModelPrice,
	OrganizationAISpendUser,
} from "#/api/typesGenerated";

/** Keys `provider|model` for every model price with a configured rate. */
export const pricedModelKeys = (
	prices: readonly AIModelPrice[],
): ReadonlySet<string> =>
	new Set(
		prices
			.filter(
				(price) => price.input_price !== null || price.output_price !== null,
			)
			.map((price) => `${price.provider}|${price.model}`),
	);

/**
 * Returns the user's models that no provider they used has a price for. The
 * report does not pair models with providers, so a model counts as priced
 * when any of the user's providers prices it.
 */
export const findUnpricedModels = (
	user: Pick<OrganizationAISpendUser, "models" | "providers">,
	pricedKeys: ReadonlySet<string>,
): string[] =>
	user.models.filter(
		(model) =>
			!user.providers.some((provider) =>
				pricedKeys.has(`${provider}|${model}`),
			),
	);
