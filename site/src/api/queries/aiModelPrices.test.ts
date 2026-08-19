import { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { UpsertAIModelPricesRequest } from "#/api/typesGenerated";
import { aiModelPrice, upsertAIModelPrices } from "./aiModelPrices";

const request: UpsertAIModelPricesRequest = {
	prices: [
		{
			provider: "openai",
			model: "custom-model",
			input_price: 2_000_000,
			output_price: null,
			cache_read_price: null,
			cache_write_price: null,
		},
	],
};

describe("AI model price queries", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("nests model price keys under the collection key", () => {
		expect(aiModelPrice("openai", "custom-model").queryKey).toEqual([
			"ai",
			"model-prices",
			"openai",
			"custom-model",
		]);
	});

	it("filters the API request by provider and model", async () => {
		const getAIModelPricesSpy = vi
			.spyOn(API.experimental, "getAIModelPrices")
			.mockResolvedValue([]);
		const queryClient = new QueryClient();

		await expect(
			queryClient.fetchQuery(aiModelPrice("openai", "custom-model")),
		).resolves.toBeUndefined();
		expect(getAIModelPricesSpy).toHaveBeenCalledWith({
			provider: "openai",
			model: "custom-model",
		});
	});

	it("invalidates the collection and updated model after an upsert", async () => {
		const queryClient = new QueryClient();
		const invalidateSpy = vi
			.spyOn(queryClient, "invalidateQueries")
			.mockResolvedValue(undefined);

		await upsertAIModelPrices(queryClient).onSuccess(request);

		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: ["ai", "model-prices"],
		});
		expect(invalidateSpy).toHaveBeenCalledWith({
			queryKey: ["ai", "model-prices", "openai", "custom-model"],
		});
	});
});
