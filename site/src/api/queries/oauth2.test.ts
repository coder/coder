import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { getSettings, oauth2ProviderAppKey, putSettings } from "./oauth2";

vi.mock("#/api/api", () => ({
	API: {
		getOAuth2ProviderSettings: vi.fn(),
		putOAuth2ProviderSettings: vi.fn(),
	},
}));

const settings: TypesGen.OAuth2ProviderSettings = {
	dynamic_client_registration_enabled: true,
};

describe("getSettings", () => {
	it("fetches settings via the API client", async () => {
		const getSettingsMock = vi.mocked(API.getOAuth2ProviderSettings);
		getSettingsMock.mockResolvedValue(settings);

		const result = await getSettings().queryFn();

		expect(getSettingsMock).toHaveBeenCalled();
		expect(result).toEqual(settings);
	});
});

describe("putSettings", () => {
	it("delegates directly to the API client", async () => {
		const putSettingsMock = vi.mocked(API.putOAuth2ProviderSettings);
		putSettingsMock.mockResolvedValue(settings);
		const queryClient = createTestQueryClient();

		const result = await putSettings(queryClient).mutationFn(settings);

		expect(putSettingsMock).toHaveBeenCalledWith(settings);
		expect(result).toEqual(settings);
	});

	// `invalidateQueries` matches by key prefix, so asserting the settings key
	// was invalidated says nothing about what else went with it. Seeding an app
	// query alongside it is what catches a widened invalidation scope, which
	// would refetch every app on every settings save.
	it("invalidates the settings query without touching app queries", async () => {
		const queryClient = createTestQueryClient();
		const settingsQueryKey = getSettings().queryKey;
		const appQueryKey = oauth2ProviderAppKey("app-1");
		queryClient.setQueryData(settingsQueryKey, {
			dynamic_client_registration_enabled: false,
		});
		queryClient.setQueryData(appQueryKey, { id: "app-1" });

		await putSettings(queryClient).onSuccess(settings);

		expect(queryClient.getQueryState(settingsQueryKey)?.isInvalidated).toBe(
			true,
		);
		expect(queryClient.getQueryState(appQueryKey)?.isInvalidated).toBe(false);
	});

	// Invalidating resolves whether or not the refetch that follows succeeds, and
	// a failed refetch keeps the query's last successful data. Seeding the cache
	// from the response the server just returned is what stops a successful save
	// from rendering the pre-save value under an error alert.
	it("writes the saved value into the cache", async () => {
		const queryClient = createTestQueryClient();
		const settingsQueryKey = getSettings().queryKey;
		queryClient.setQueryData(settingsQueryKey, {
			dynamic_client_registration_enabled: false,
		});

		await putSettings(queryClient).onSuccess({
			dynamic_client_registration_enabled: true,
		});

		expect(queryClient.getQueryData(settingsQueryKey)).toEqual({
			dynamic_client_registration_enabled: true,
		});
	});
});
