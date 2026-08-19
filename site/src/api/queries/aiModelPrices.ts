import { type QueryClient, queryOptions } from "react-query";
import { API } from "#/api/api";
import type {
	AIModelPrice,
	UpsertAIModelPricesRequest,
} from "#/api/typesGenerated";

const aiModelPricesKey = ["ai", "model-prices"] as const;

const aiModelPriceKey = (provider: string, model: string) =>
	[...aiModelPricesKey, provider, model] as const;

export const aiModelPrice = (provider: string, model: string) =>
	queryOptions({
		queryKey: aiModelPriceKey(provider, model),
		queryFn: async (): Promise<AIModelPrice | undefined> => {
			const prices = await API.experimental.getAIModelPrices({
				provider,
				model,
			});
			return prices.at(0);
		},
	});

export const upsertAIModelPrices = (queryClient: QueryClient) => ({
	mutationFn: async (
		request: UpsertAIModelPricesRequest,
	): Promise<UpsertAIModelPricesRequest> => {
		await API.experimental.upsertAIModelPrices(request);
		return request;
	},
	onSuccess: async (request: UpsertAIModelPricesRequest) => {
		await Promise.all([
			queryClient.invalidateQueries({ queryKey: aiModelPricesKey }),
			...request.prices.map((price) =>
				queryClient.invalidateQueries({
					queryKey: aiModelPriceKey(price.provider, price.model),
				}),
			),
		]);
	},
});
