import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	getOAuth2ProviderSettings,
	oauth2ProviderAppKey,
	putOAuth2ProviderSettings,
} from "./oauth2";

vi.mock("#/api/api", () => ({
	API: {
		getOAuth2ProviderSettings: vi.fn(),
		putOAuth2ProviderSettings: vi.fn(),
	},
}));

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

	// `invalidateQueries` matches by key prefix, so asserting the settings key
	// was invalidated says nothing about what else went with it. Seeding an app
	// query alongside it is what catches a widened invalidation scope, which
	// would refetch every app on every settings save.
	it("invalidates the settings query without touching app queries", async () => {
		const queryClient = createTestQueryClient();
		const settingsQueryKey = getOAuth2ProviderSettings().queryKey;
		const appQueryKey = oauth2ProviderAppKey("app-1");
		queryClient.setQueryData(settingsQueryKey, {
			dynamic_client_registration_enabled: false,
		});
		queryClient.setQueryData(appQueryKey, { id: "app-1" });

		await putOAuth2ProviderSettings(queryClient).onSuccess();

		expect(queryClient.getQueryState(settingsQueryKey)?.isInvalidated).toBe(
			true,
		);
		expect(queryClient.getQueryState(appQueryKey)?.isInvalidated).toBe(false);
	});
});
