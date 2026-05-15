import { QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import type {
	UpdateUserAppearanceSettingsRequest,
	UserAppearanceSettings,
} from "#/api/typesGenerated";
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

		const mutation = updateAppearanceSettings(queryClient);
		const context = await mutation.onMutate?.(optimisticSettings);
		expect(queryClient.getQueryData(myAppearanceKey)).toEqual(
			optimisticSettings,
		);

		mutation.onError?.(new Error("failed"), optimisticSettings, context);

		expect(queryClient.getQueryData(myAppearanceKey)).toEqual(previousSettings);
	});

	it("removes optimistic appearance data when rollback has no prior cache", async () => {
		const queryClient = new QueryClient();
		const optimisticSettings = updateRequest();
		const mutation = updateAppearanceSettings(queryClient);

		const context = await mutation.onMutate?.(optimisticSettings);
		expect(queryClient.getQueryData(myAppearanceKey)).toEqual(
			optimisticSettings,
		);

		mutation.onError?.(new Error("failed"), optimisticSettings, context);

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
		const mutation = updateAppearanceSettings(queryClient);

		const context = await mutation.onMutate?.(optimisticSettings);
		if (!context) {
			throw new Error("expected mutation context");
		}
		expect(queryClient.getQueryData(myAppearanceKey)).toEqual(
			optimisticSettings,
		);

		mutation.onSuccess?.(serverSettings, optimisticSettings, context);

		expect(queryClient.getQueryData(myAppearanceKey)).toEqual(serverSettings);
	});

	it("keeps patch values when a successful appearance update response is partial", async () => {
		const queryClient = new QueryClient();
		const optimisticSettings = updateRequest({
			theme_mode: "sync",
			theme_light: "light-protan-deuter",
			theme_dark: "dark-protan-deuter",
		});
		const serverSettings = {
			theme_preference: "dark-tritan",
			terminal_font: "jetbrains-mono",
		} satisfies Partial<UserAppearanceSettings>;
		const mutation = updateAppearanceSettings(queryClient);

		const context = await mutation.onMutate?.(optimisticSettings);
		if (!context) {
			throw new Error("expected mutation context");
		}

		mutation.onSuccess?.(
			serverSettings as UserAppearanceSettings,
			optimisticSettings,
			context,
		);

		expect(queryClient.getQueryData(myAppearanceKey)).toEqual({
			...optimisticSettings,
			...serverSettings,
		});
	});
});
