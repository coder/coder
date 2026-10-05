import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	focusManager,
	type QueryClient,
	QueryClientProvider,
} from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatProjectInstructionsKey } from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectInstructions,
} from "#/api/typesGenerated";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import {
	MockChatProject,
	MockChatProjectInstructions,
	MockUnsetChatProjectInstructions,
	mockApiError,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { ProjectDetailsPanel } from "./ProjectDetailsPanel";

const mockInstructions = (instructions: ChatProjectInstructions) =>
	vi
		.spyOn(API.experimental, "getChatProjectInstructions")
		.mockResolvedValue(instructions);

/** Simulates a focus refetch that returns instructions saved elsewhere. */
const refetchWith = async (
	getInstructions: ReturnType<typeof mockInstructions>,
	queryClient: QueryClient,
	instructions: string,
) => {
	getInstructions.mockResolvedValue({
		...MockChatProjectInstructions,
		instructions,
	});
	act(() => {
		focusManager.setFocused(false);
		focusManager.setFocused(true);
	});
	await waitFor(() =>
		expect(
			queryClient.getQueryData(chatProjectInstructionsKey(MockChatProject.id)),
		).toMatchObject({ instructions }),
	);
};

const renderPanel = (project: ChatProject = MockChatProject) => {
	const queryClient = createTestQueryClient();
	const panel = (project: ChatProject) => (
		<ThemeOverride theme={themes[DEFAULT_THEME]}>
			<QueryClientProvider client={queryClient}>
				<ProjectDetailsPanel project={project} />
			</QueryClientProvider>
		</ThemeOverride>
	);
	const { rerender } = render(panel(project));
	return {
		user: userEvent.setup(),
		queryClient,
		rerenderWithProject: (project: ChatProject) => rerender(panel(project)),
	};
};

describe("ProjectDetailsPanel", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		focusManager.setFocused(undefined);
	});

	it("creates instructions and shows the saved text without refetching", async () => {
		const text = "Use TypeScript for new code.";
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockResolvedValue({
				...MockChatProjectInstructions,
				instructions: text,
			});
		const getInstructions = mockInstructions(MockUnsetChatProjectInstructions);
		const { user, queryClient } = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Create" }));
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			text,
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(updateInstructions).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
			{ instructions: text },
		);
		// The PUT response fills the cache, so the reopened editor reflects
		// the saved instructions without another GET.
		await waitFor(() =>
			expect(
				queryClient.getQueryData(
					chatProjectInstructionsKey(MockChatProject.id),
				),
			).toEqual({ ...MockChatProjectInstructions, instructions: text }),
		);
		await user.click(
			await screen.findByRole("button", { name: "Edit instructions" }),
		);
		expect(screen.getByRole("textbox", { name: "Instructions" })).toHaveValue(
			text,
		);
		expect(getInstructions).toHaveBeenCalledTimes(1);
	});

	it("edits the saved instructions", async () => {
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockResolvedValue({
				...MockChatProjectInstructions,
				instructions: "Write tests first.",
			});
		mockInstructions(MockChatProjectInstructions);
		const { user } = renderPanel();

		await user.click(
			await screen.findByRole("button", { name: "Edit instructions" }),
		);
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		expect(textbox).toHaveValue(MockChatProjectInstructions.instructions);
		await user.clear(textbox);
		await user.type(textbox, "Write tests first.");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(updateInstructions).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
			{ instructions: "Write tests first." },
		);
	});

	it("closes the editor and discards the draft when the project changes", async () => {
		mockInstructions(MockChatProjectInstructions);
		const { user, rerenderWithProject } = renderPanel();

		await user.click(
			await screen.findByRole("button", { name: "Edit instructions" }),
		);
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			" Draft for the first project.",
		);
		rerenderWithProject({
			...MockChatProject,
			id: "00000000-0000-4000-8000-0000000000b2",
		});

		// The open editor would hide the panel's Edit button from queries.
		await user.click(
			await screen.findByRole("button", { name: "Edit instructions" }),
		);
		expect(screen.getByRole("textbox", { name: "Instructions" })).toHaveValue(
			MockChatProjectInstructions.instructions,
		);
	});

	it("deletes the instructions and returns to the empty state", async () => {
		const deleteInstructions = vi
			.spyOn(API.experimental, "deleteChatProjectInstructions")
			.mockResolvedValue();
		mockInstructions(MockChatProjectInstructions);
		const { user, queryClient } = renderPanel();

		await user.click(
			await screen.findByRole("button", { name: "Edit instructions" }),
		);
		await user.click(screen.getByRole("button", { name: "Delete" }));

		expect(deleteInstructions).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
		);
		await waitFor(() =>
			expect(
				queryClient.getQueryData(
					chatProjectInstructionsKey(MockChatProject.id),
				),
			).toEqual(MockUnsetChatProjectInstructions),
		);
	});

	it("keeps the editor open with the draft when saving fails", async () => {
		vi.spyOn(
			API.experimental,
			"updateChatProjectInstructions",
		).mockRejectedValue(
			mockApiError({
				message: "Instructions exceed maximum length.",
				detail: "Maximum length is 131072 bytes, got 140000.",
			}),
		);
		mockInstructions(MockUnsetChatProjectInstructions);
		const { user } = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Create" }));
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		await user.type(textbox, "Too long");
		await user.click(screen.getByRole("button", { name: "Save" }));

		const dialog = screen.getByRole("dialog");
		// The detail names the limit the user has to stay under.
		await waitFor(() =>
			expect(dialog).toHaveTextContent("Instructions exceed maximum length."),
		);
		expect(dialog).toHaveTextContent(
			"Maximum length is 131072 bytes, got 140000.",
		);
		expect(textbox).toHaveValue("Too long");

		// Editing the draft dismisses the error about the old text.
		await user.type(textbox, "!");
		expect(dialog).not.toHaveTextContent("Instructions exceed maximum length.");
	});

	it("shows the error of the latest failed action", async () => {
		vi.spyOn(
			API.experimental,
			"updateChatProjectInstructions",
		).mockRejectedValue(mockApiError({ message: "Failed to save." }));
		vi.spyOn(
			API.experimental,
			"deleteChatProjectInstructions",
		).mockRejectedValue(mockApiError({ message: "Failed to delete." }));
		mockInstructions(MockChatProjectInstructions);
		const { user } = renderPanel();

		await user.click(
			await screen.findByRole("button", { name: "Edit instructions" }),
		);
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			" More.",
		);
		const dialog = screen.getByRole("dialog");
		await user.click(screen.getByRole("button", { name: "Save" }));
		await waitFor(() => expect(dialog).toHaveTextContent("Failed to save."));
		await user.click(screen.getByRole("button", { name: "Delete" }));

		await waitFor(() => expect(dialog).toHaveTextContent("Failed to delete."));
		expect(dialog).not.toHaveTextContent("Failed to save.");
	});

	it("sends one update when Save is double clicked", async () => {
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockReturnValue(new Promise(() => {}));
		mockInstructions(MockUnsetChatProjectInstructions);
		const { user } = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Create" }));
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			"Be brief.",
		);
		// With no delay between the clicks, the second lands before the
		// pending state can disable Save, like a fast real double click.
		await userEvent
			.setup({ delay: null })
			.dblClick(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(updateInstructions).toHaveBeenCalled());
		expect(updateInstructions).toHaveBeenCalledTimes(1);
	});

	it("refetches the instructions when the window regains focus", async () => {
		const getInstructions = mockInstructions(MockChatProjectInstructions);
		const { user, queryClient } = renderPanel();
		await screen.findByRole("button", { name: "Edit instructions" });

		// Another editor changed the instructions while this tab was hidden.
		await refetchWith(getInstructions, queryClient, "Edited in another tab.");

		await user.click(
			await screen.findByRole("button", { name: "Edit instructions" }),
		);
		await waitFor(() =>
			expect(screen.getByRole("textbox", { name: "Instructions" })).toHaveValue(
				"Edited in another tab.",
			),
		);
	});

	it("blocks Save until the user resolves a refetch that changed the instructions", async () => {
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockResolvedValue(MockChatProjectInstructions);
		const getInstructions = mockInstructions(MockChatProjectInstructions);
		const { user, queryClient } = renderPanel();
		await user.click(
			await screen.findByRole("button", { name: "Edit instructions" }),
		);
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		await user.type(textbox, " My edit.");

		// Another editor changed the instructions while this one was open.
		await refetchWith(getInstructions, queryClient, "Edited in another tab.");

		const saveButton = screen.getByRole("button", { name: "Save" });
		await user.type(textbox, " More.");
		await user.click(saveButton);
		expect(updateInstructions).not.toHaveBeenCalled();

		await user.click(screen.getByRole("button", { name: "Keep my draft" }));
		await user.click(saveButton);
		expect(updateInstructions).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
			{
				instructions: `${MockChatProjectInstructions.instructions} My edit. More.`,
			},
		);
	});

	it.each([
		{
			action: "Save",
			result: { ...MockChatProjectInstructions, instructions: "Saved text." },
		},
		{ action: "Delete", result: MockUnsetChatProjectInstructions },
	])(
		"keeps the result of $action when a stale refetch finishes after it",
		async ({ action, result }) => {
			vi.spyOn(
				API.experimental,
				"updateChatProjectInstructions",
			).mockResolvedValue(result);
			vi.spyOn(
				API.experimental,
				"deleteChatProjectInstructions",
			).mockResolvedValue();
			const getInstructions = mockInstructions(MockChatProjectInstructions);
			const { user, queryClient } = renderPanel();
			await user.click(
				await screen.findByRole("button", { name: "Edit instructions" }),
			);
			const textbox = screen.getByRole("textbox", { name: "Instructions" });
			await user.clear(textbox);
			await user.type(textbox, "Saved text.");

			// A focus refetch reads the old instructions but answers late.
			let resolveStaleFetch: (value: ChatProjectInstructions) => void =
				() => {};
			getInstructions.mockReturnValue(
				new Promise((resolve) => {
					resolveStaleFetch = resolve;
				}),
			);
			act(() => {
				focusManager.setFocused(false);
				focusManager.setFocused(true);
			});
			await waitFor(() => expect(getInstructions).toHaveBeenCalledTimes(2));
			await user.click(screen.getByRole("button", { name: action }));
			await waitFor(() =>
				expect(
					queryClient.getQueryData(
						chatProjectInstructionsKey(MockChatProject.id),
					),
				).toEqual(result),
			);

			await act(async () => resolveStaleFetch(MockChatProjectInstructions));
			expect(
				queryClient.getQueryData(
					chatProjectInstructionsKey(MockChatProject.id),
				),
			).toEqual(result);
		},
	);

	it("retries loading the instructions after an error", async () => {
		const getInstructions = vi
			.spyOn(API.experimental, "getChatProjectInstructions")
			.mockRejectedValueOnce(mockApiError({ message: "Failed to load." }))
			.mockResolvedValue(MockChatProjectInstructions);
		const { user } = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Retry" }));

		await waitFor(() => expect(getInstructions).toHaveBeenCalledTimes(2));
		await screen.findByRole("button", { name: "Edit instructions" });
	});
});
