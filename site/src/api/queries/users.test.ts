import { MutationObserver, QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import { API } from "#/api/api";
import type {
	UpdateUserAppearanceSettingsRequest,
	UserAppearanceSettings,
} from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import { myAppearanceKey, updateAppearanceSettings } from "./users";

const appearanceSettings = (
	overrides: Partial<UserAppearanceSettings> = {},
): UserAppearanceSettings => ({
	theme_preference: "dark-tritan",
	theme_mode: "sync",
	theme_light: "light-tritan",
	theme_dark: "dark-tritan",
	terminal_font: "geist-mono",
	...overrides,
});

const updateRequest = (
	overrides: Partial<UpdateUserAppearanceSettingsRequest> = {},
): UpdateUserAppearanceSettingsRequest => ({
	theme_preference: "dark",
	theme_mode: "single",
	theme_light: "light-tritan",
	theme_dark: "dark-tritan",
	terminal_font: "fira-code",
	...overrides,
});

const startAppearanceUpdate = (
	queryClient: QueryClient,
	request: UpdateUserAppearanceSettingsRequest,
) => {
	const response = createDeferred<UserAppearanceSettings>();
	vi.spyOn(API, "updateAppearanceSettings").mockReturnValue(response.promise);
	const observer = new MutationObserver(
		queryClient,
		updateAppearanceSettings(queryClient),
	);

	return { response, result: observer.mutate(request) };
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("updateAppearanceSettings", () => {
	it("rolls back optimistic appearance updates when the mutation fails", async () => {
		const queryClient = new QueryClient();
		const previousSettings = appearanceSettings({
			theme_light: "light-protan-deuter",
			theme_dark: "dark-protan-deuter",
		});
		const optimisticSettings = updateRequest();

		queryClient.setQueryData<UserAppearanceSettings>(
			myAppearanceKey,
			previousSettings,
		);

		const { response, result } = startAppearanceUpdate(
			queryClient,
			optimisticSettings,
		);
		await vi.waitFor(() =>
			expect(queryClient.getQueryData(myAppearanceKey)).toEqual(
				optimisticSettings,
			),
		);

		const rejection = expect(result).rejects.toThrow("failed");
		response.reject(new Error("failed"));
		await rejection;

		expect(queryClient.getQueryData(myAppearanceKey)).toEqual(previousSettings);
	});

	it("removes optimistic appearance data when rollback has no prior cache", async () => {
		const queryClient = new QueryClient();
		const optimisticSettings = updateRequest();
		const { response, result } = startAppearanceUpdate(
			queryClient,
			optimisticSettings,
		);

		await vi.waitFor(() =>
			expect(queryClient.getQueryData(myAppearanceKey)).toEqual(
				optimisticSettings,
			),
		);

		const rejection = expect(result).rejects.toThrow("failed");
		response.reject(new Error("failed"));
		await rejection;

		expect(queryClient.getQueryData(myAppearanceKey)).toBeUndefined();
	});

	it("stores the server response after a successful appearance update", async () => {
		const queryClient = new QueryClient();
		const optimisticSettings = updateRequest();
		const serverSettings = appearanceSettings({
			theme_preference: "dark-protan-deuter",
			theme_light: "light-protan-deuter",
			theme_dark: "dark-protan-deuter",
		});
		const { response, result } = startAppearanceUpdate(
			queryClient,
			optimisticSettings,
		);

		await vi.waitFor(() =>
			expect(queryClient.getQueryData(myAppearanceKey)).toEqual(
				optimisticSettings,
			),
		);

		response.resolve(serverSettings);
		await result;

		expect(queryClient.getQueryData(myAppearanceKey)).toEqual(serverSettings);
	});
});
