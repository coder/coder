import { renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { useModelOptions } from "./useModelOptions";

const createWrapper = (): FC<PropsWithChildren> => {
	const queryClient = createTestQueryClient();
	return ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
};

const catalog: TypesGen.ChatModelsResponse = {
	providers: [{ provider: "openai", available: true, models: [] }],
	unsupported_providers: [],
};

const modelConfigs: TypesGen.ChatModelConfig[] = [
	{
		id: "config-openai",
		ai_provider_id: "prov-openai",
		model: "gpt-4o",
		display_name: "GPT-4o",
		enabled: true,
		is_default: false,
		context_limit: 128_000,
		compression_threshold: 70,
		created_at: "2026-02-18T00:00:00.000Z",
		updated_at: "2026-02-18T00:00:00.000Z",
	},
];

const userProviderConfigs: TypesGen.UserAIProviderKeyConfig[] = [
	{
		provider: {
			id: "prov-openai",
			type: "openai",
			name: "openai",
			display_name: "OpenAI",
			enabled: true,
			deleted: false,
		},
		has_user_api_key: false,
		has_provider_api_key: true,
		byok_enabled: true,
	},
];

describe("useModelOptions", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("stays loading and drops options while the provider query is pending", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue(catalog);
		vi.spyOn(API.experimental, "getChatModelConfigs").mockResolvedValue(
			modelConfigs,
		);
		// The provider query never resolves: catalog + configs settle first,
		// which is exactly the race that previously dropped every option and
		// flashed "No Models".
		vi.spyOn(API.experimental, "getUserAIProviderKeyConfigs").mockReturnValue(
			new Promise<TypesGen.UserAIProviderKeyConfig[]>(() => {}),
		);

		const { result } = renderHook(() => useModelOptions(), {
			wrapper: createWrapper(),
		});

		// Wait until catalog + configs have resolved so the pending provider
		// query is the only thing still loading.
		await waitFor(() => {
			expect(result.current.modelCatalog).toEqual(catalog);
		});

		expect(result.current.isModelCatalogLoading).toBe(true);
		expect(result.current.options).toEqual([]);
	});

	it("resolves options once every query settles", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue(catalog);
		vi.spyOn(API.experimental, "getChatModelConfigs").mockResolvedValue(
			modelConfigs,
		);
		vi.spyOn(API.experimental, "getUserAIProviderKeyConfigs").mockResolvedValue(
			userProviderConfigs,
		);

		const { result } = renderHook(() => useModelOptions(), {
			wrapper: createWrapper(),
		});

		await waitFor(() => {
			expect(result.current.isModelCatalogLoading).toBe(false);
		});

		expect(result.current.options).toEqual([
			{
				id: "config-openai",
				provider: "openai",
				model: "gpt-4o",
				displayName: "GPT-4o",
				contextLimit: 128_000,
			},
		]);
	});
});
