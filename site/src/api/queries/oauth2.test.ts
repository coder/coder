import { QueryClient } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { getOAuth2ProviderSettings, putOAuth2ProviderSettings } from "./oauth2";

vi.mock("#/api/api", () => ({
	API: {
		getOAuth2ProviderSettings: vi.fn(),
		putOAuth2ProviderSettings: vi.fn(),
	},
}));

const createTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: Number.POSITIVE_INFINITY,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
			},
		},
	});

const settings: TypesGen.OAuth2ProviderSettings = {
	dynamic_client_registration_enabled: true,
};

describe("getOAuth2ProviderSettings", () => {
	it("uses a queryKey nested under the oauth2-provider prefix", () => {
		expect(getOAuth2ProviderSettings().queryKey).toEqual([
			"oauth2-provider",
			"settings",
		]);
	});

	it("fetches settings via the API client", async () => {
		const getSettingsMock = vi.mocked(API.getOAuth2ProviderSettings);
		getSettingsMock.mockResolvedValue(settings);

		const result = await getOAuth2ProviderSettings().queryFn();

		expect(getSettingsMock).toHaveBeenCalled();
		expect(result).toEqual(settings);
	});
});

describe("putOAuth2ProviderSettings", () => {
	it("delegates directly to the API client", async () => {
		const putSettingsMock = vi.mocked(API.putOAuth2ProviderSettings);
		putSettingsMock.mockResolvedValue(settings);
		const queryClient = createTestQueryClient();

		const result =
			await putOAuth2ProviderSettings(queryClient).mutationFn(settings);

		expect(putSettingsMock).toHaveBeenCalledWith(settings);
		expect(result).toEqual(settings);
	});

	it("invalidates the settings query on a successful update", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(getOAuth2ProviderSettings().queryKey, {
			dynamic_client_registration_enabled: false,
		});

		await putOAuth2ProviderSettings(queryClient).onSuccess();

		expect(
			queryClient.getQueryState(getOAuth2ProviderSettings().queryKey)
				?.isInvalidated,
		).toBe(true);
	});
});
