import { QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type {
	UpdateUserAppearanceSettingsRequest,
	UserAppearanceSettings,
} from "#/api/typesGenerated";
import {
	defaultMetadataManager,
	MetadataManager,
} from "#/hooks/useEmbeddedMetadata";
import {
	MockExperiments,
	MockUserOwner,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import {
	logout,
	myAppearanceKey,
	preferenceSettings,
	updateAppearanceSettings,
} from "./users";

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

describe("logout", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("clears embedded account data so another session fetches its own preferences", async () => {
		const metadata = {
			user: MockUserOwner,
			experiments: MockExperiments,
			userPreferences: {
				...MockUserPreferenceSettings,
				default_landing_page: "agents",
			},
		};
		const nodes = Object.entries(metadata).map(([key, value]) => {
			const node = document.createElement("meta");
			node.setAttribute("data-logout-metadata", key);
			node.setAttribute("content", JSON.stringify(value));
			document.head.append(node);
			return node;
		});
		const manager = new MetadataManager("data-logout-metadata");
		vi.spyOn(defaultMetadataManager, "getMetadata").mockImplementation(
			manager.getMetadata,
		);
		vi.spyOn(defaultMetadataManager, "clearMetadataByKey").mockImplementation(
			manager.clearMetadataByKey,
		);
		const queryClient = new QueryClient();

		try {
			expect(await queryClient.ensureQueryData(preferenceSettings())).toEqual(
				metadata.userPreferences,
			);
			logout(queryClient).onSettled?.(undefined, null, undefined, undefined);

			const cleared = manager.getMetadata();
			expect(cleared.user.available).toBe(false);
			expect(cleared.experiments.available).toBe(false);
			expect(cleared.userPreferences.available).toBe(false);

			const getPreference = vi
				.spyOn(API, "getUserPreferenceSettings")
				.mockResolvedValue(MockUserPreferenceSettings);
			expect(await queryClient.ensureQueryData(preferenceSettings())).toEqual(
				MockUserPreferenceSettings,
			);
			expect(getPreference).toHaveBeenCalledTimes(1);
		} finally {
			queryClient.clear();
			for (const node of nodes) {
				node.remove();
			}
		}
	});
});
