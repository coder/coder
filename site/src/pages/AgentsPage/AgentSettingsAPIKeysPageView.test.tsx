import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type {
	UserAIProviderKeyConfig,
	UserChatProviderConfig,
} from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	AgentSettingsAPIKeysPageView,
	type AgentSettingsAPIKeysPageViewProps,
} from "./AgentSettingsAPIKeysPageView";

const createProvider = (
	overrides: Partial<UserChatProviderConfig> &
		Pick<UserChatProviderConfig, "provider_id" | "provider">,
): UserChatProviderConfig => ({
	provider_id: overrides.provider_id,
	provider: overrides.provider,
	display_name: overrides.display_name ?? overrides.provider,
	icon: overrides.icon ?? "",
	enabled: overrides.enabled ?? true,
	has_user_api_key: overrides.has_user_api_key ?? false,
	has_central_api_key_fallback: overrides.has_central_api_key_fallback ?? false,
	byok_enabled: overrides.byok_enabled ?? true,
});

const baseProvider = createProvider({
	provider_id: "prov-1",
	provider: "openai",
	display_name: "OpenAI",
});

const savedKeyConfig: UserAIProviderKeyConfig = {
	provider: {
		id: "prov-1",
		type: "openai",
		name: "openai",
		display_name: "OpenAI",
		icon: "",
		enabled: true,
		deleted: false,
	},
	has_user_api_key: true,
	has_provider_api_key: false,
	byok_enabled: true,
};

const defaultProps: AgentSettingsAPIKeysPageViewProps = {
	error: undefined,
	isLoading: false,
	providers: [baseProvider],
	models: [],
	isModelsLoading: false,
	areModelsUnavailable: false,
};

const renderView = (ui: ReactElement) => {
	const queryClient = createTestQueryClient();
	queryClient.setDefaultOptions({
		...queryClient.getDefaultOptions(),
		mutations: { retry: false },
	});
	return render(
		<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>,
	);
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("AgentSettingsAPIKeysPageView", () => {
	it("saves a replacement key after a successful save remasks the field", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "upsertUserAIProviderKey").mockResolvedValue(
			savedKeyConfig,
		);

		renderView(<AgentSettingsAPIKeysPageView {...defaultProps} />);

		const apiKeyInput = screen.getByLabelText("API Key");
		await user.type(apiKeyInput, "sk-test-key");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(API.experimental.upsertUserAIProviderKey).toHaveBeenCalledWith(
				"prov-1",
				{ api_key: "sk-test-key" },
			);
		});

		await user.type(apiKeyInput, "sk-other-key");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(API.experimental.upsertUserAIProviderKey).toHaveBeenCalledTimes(2);
		});
		expect(API.experimental.upsertUserAIProviderKey).toHaveBeenLastCalledWith(
			"prov-1",
			{ api_key: "sk-other-key" },
		);
	});

	it("trims leading and trailing whitespace before saving", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "upsertUserAIProviderKey").mockResolvedValue(
			savedKeyConfig,
		);

		renderView(<AgentSettingsAPIKeysPageView {...defaultProps} />);

		await user.type(screen.getByLabelText("API Key"), "  sk-test-key  ");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(API.experimental.upsertUserAIProviderKey).toHaveBeenCalledWith(
				"prov-1",
				{ api_key: "sk-test-key" },
			);
		});
	});

	it("keeps the draft after a failed save so the user can retry", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "upsertUserAIProviderKey")
			.mockRejectedValueOnce(new Error("failed to save"))
			.mockResolvedValueOnce(savedKeyConfig);

		renderView(<AgentSettingsAPIKeysPageView {...defaultProps} />);

		await user.type(screen.getByLabelText("API Key"), "sk-test-key");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(API.experimental.upsertUserAIProviderKey).toHaveBeenCalledTimes(1);
		});

		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(API.experimental.upsertUserAIProviderKey).toHaveBeenCalledTimes(2);
		});
		expect(API.experimental.upsertUserAIProviderKey).toHaveBeenLastCalledWith(
			"prov-1",
			{ api_key: "sk-test-key" },
		);
	});

	it("calls the remove API when the user confirms removal", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "deleteUserAIProviderKey").mockResolvedValue(
			undefined,
		);

		renderView(
			<AgentSettingsAPIKeysPageView
				{...defaultProps}
				providers={[
					{
						...baseProvider,
						has_user_api_key: true,
					},
				]}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Remove" }));
		const dialog = await screen.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Remove" }));

		await waitFor(() => {
			expect(API.experimental.deleteUserAIProviderKey).toHaveBeenCalledWith(
				"prov-1",
			);
		});
	});

	it("keeps the remove dialog open after a failed remove so the user can retry", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "deleteUserAIProviderKey")
			.mockRejectedValueOnce(new Error("failed to remove"))
			.mockResolvedValueOnce(undefined);

		renderView(
			<AgentSettingsAPIKeysPageView
				{...defaultProps}
				providers={[
					{
						...baseProvider,
						has_user_api_key: true,
					},
				]}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Remove" }));
		const dialog = await screen.findByRole("dialog");
		const confirmButton = within(dialog).getByRole("button", {
			name: "Remove",
		});
		await user.click(confirmButton);

		await waitFor(() => {
			expect(API.experimental.deleteUserAIProviderKey).toHaveBeenCalledTimes(1);
		});

		await user.click(confirmButton);

		await waitFor(() => {
			expect(API.experimental.deleteUserAIProviderKey).toHaveBeenCalledTimes(2);
		});
	});
});
