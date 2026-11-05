import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { API } from "#/api/api";
import { chatUserCustomPrompt } from "#/api/queries/chats";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { UserChatCustomPrompt } from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import AgentSettingsGeneralPage from "../AgentSettingsPage/AgentSettingsGeneralPage/AgentSettingsGeneralPage";

const renderPage = (custom_prompt?: string) => {
	const queryClient = createTestQueryClient();
	if (custom_prompt !== undefined) {
		queryClient.setQueryData(chatUserCustomPrompt().queryKey, {
			custom_prompt,
		});
	}
	queryClient.setQueryData(preferenceSettingsKey, MockUserPreferenceSettings);
	renderComponent(
		<QueryClientProvider client={queryClient}>
			<AgentSettingsGeneralPage />
		</QueryClientProvider>,
	);
	return queryClient;
};

beforeEach(() => {
	vi.spyOn(API.experimental, "updateUserChatCustomPrompt").mockImplementation(
		async (values) => values,
	);
	vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
		MockUserPreferenceSettings,
	);
	vi.spyOn(API.experimental, "getUserChatDebugLogging").mockResolvedValue({
		debug_logging_enabled: false,
		user_toggle_allowed: false,
		forced_by_deployment: false,
	});
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("PersonalInstructionsSettings", () => {
	it.each(["Existing saved instructions", ""])(
		"blocks editing until the initial prompt %j loads, then saves edits",
		async (custom_prompt) => {
			const user = userEvent.setup();
			const pending = createDeferred<UserChatCustomPrompt>();
			vi.spyOn(API.experimental, "getUserChatCustomPrompt").mockReturnValue(
				pending.promise,
			);
			const queryClient = renderPage();
			const textarea = screen.getByRole("textbox", {
				name: "Personal instructions",
			});
			await user.type(textarea, "Premature draft");
			expect(textarea).toHaveValue("");
			expect(
				API.experimental.updateUserChatCustomPrompt,
			).not.toHaveBeenCalled();

			await act(async () => pending.resolve({ custom_prompt }));
			await waitFor(() =>
				expect(
					queryClient.getQueryState(chatUserCustomPrompt().queryKey)?.status,
				).toBe("success"),
			);
			await user.type(textarea, " and my edits");
			await user.click(screen.getByRole("button", { name: "Save" }));
			await waitFor(() =>
				expect(
					API.experimental.updateUserChatCustomPrompt,
				).toHaveBeenCalledWith({
					custom_prompt: `${custom_prompt} and my edits`,
				}),
			);
		},
	);

	it("blocks editing after an initial failure", async () => {
		vi.spyOn(API.experimental, "getUserChatCustomPrompt").mockRejectedValue(
			new Error("Unable to load personal instructions"),
		);
		const queryClient = renderPage();
		await waitFor(() =>
			expect(
				queryClient.getQueryState(chatUserCustomPrompt().queryKey)?.status,
			).toBe("error"),
		);
		const textarea = screen.getByRole("textbox", {
			name: "Personal instructions",
		});
		await userEvent.type(textarea, "Premature draft");
		expect(textarea).toHaveValue("");
		expect(API.experimental.updateUserChatCustomPrompt).not.toHaveBeenCalled();
	});

	it.each(["typed", "cleared"])(
		"preserves %s dirty edits through a background query update",
		async (draft) => {
			const user = userEvent.setup();
			const load = vi
				.spyOn(API.experimental, "getUserChatCustomPrompt")
				.mockResolvedValue({ custom_prompt: "Saved instructions" });
			const queryClient = renderPage("Saved instructions");
			const textarea = screen.getByRole("textbox", {
				name: "Personal instructions",
			});
			await user.clear(textarea);
			await user.type(textarea, "My draft");
			if (draft === "cleared") {
				await user.click(screen.getByRole("button", { name: "Clear" }));
			}
			load.mockResolvedValue({ custom_prompt: "Updated elsewhere" });
			await act(async () => {
				await queryClient.refetchQueries(chatUserCustomPrompt());
			});
			await user.type(textarea, " with more edits");
			await user.click(screen.getByRole("button", { name: "Save" }));
			await waitFor(() =>
				expect(
					API.experimental.updateUserChatCustomPrompt,
				).toHaveBeenCalledWith({
					custom_prompt: `${draft === "typed" ? "My draft" : ""} with more edits`,
				}),
			);
		},
	);

	it.each(["success", "failure"])(
		"keeps the server-confirmed save when the follow-up GET is a %s",
		async (outcome) => {
			const user = userEvent.setup();
			const load = vi
				.spyOn(API.experimental, "getUserChatCustomPrompt")
				.mockResolvedValue({ custom_prompt: "Original instructions" });
			vi.mocked(API.experimental.updateUserChatCustomPrompt).mockResolvedValue({
				custom_prompt: "Clean instructions",
			});
			const queryClient = renderPage("Original instructions");
			const textarea = screen.getByRole("textbox", {
				name: "Personal instructions",
			});
			await user.clear(textarea);
			await user.type(textarea, "Clean\u200b instructions");
			if (outcome === "failure") {
				load.mockRejectedValue(new Error("Refresh failed"));
			} else {
				load.mockResolvedValue({ custom_prompt: "Clean instructions" });
			}
			await user.click(screen.getByRole("button", { name: "Save" }));
			await waitFor(() =>
				expect(
					API.experimental.updateUserChatCustomPrompt,
				).toHaveBeenCalledWith({ custom_prompt: "Clean\u200b instructions" }),
			);
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
			expect(queryClient.getQueryData(chatUserCustomPrompt().queryKey)).toEqual(
				{
					custom_prompt: "Clean instructions",
				},
			);
			expect(textarea).toHaveValue("Clean instructions");
		},
	);
});
