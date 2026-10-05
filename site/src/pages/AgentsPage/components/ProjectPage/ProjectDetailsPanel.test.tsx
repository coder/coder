import {
	act,
	fireEvent,
	render,
	screen,
	waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { focusManager, QueryClientProvider } from "react-query";
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
		const saveButton = screen.getByRole("button", { name: "Save" });
		expect(saveButton).toBeDisabled();
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			text,
		);
		await user.click(saveButton);

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
		await user.click(await screen.findByRole("button", { name: "Edit" }));
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

		await user.click(await screen.findByRole("button", { name: "Edit" }));
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		expect(textbox).toHaveValue(MockChatProjectInstructions.instructions);
		const saveButton = screen.getByRole("button", { name: "Save" });
		expect(saveButton).toBeDisabled();
		// Surrounding whitespace alone is not a change worth saving.
		await user.type(textbox, "  ");
		expect(saveButton).toBeDisabled();
		// Neither are invisible characters, which the server strips.
		await user.type(textbox, "\u200B");
		expect(saveButton).toBeDisabled();
		// Blank instructions are deleted rather than saved.
		await user.clear(textbox);
		expect(saveButton).toBeDisabled();
		await user.type(textbox, "Write tests first.");
		await user.click(saveButton);

		expect(updateInstructions).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
			{ instructions: "Write tests first." },
		);
	});

	it("does not save text that is blank once invisible characters are stripped", async () => {
		mockInstructions(MockUnsetChatProjectInstructions);
		const { user } = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Create" }));
		fireEvent.change(screen.getByRole("textbox", { name: "Instructions" }), {
			target: { value: "\u200B\u2060 \u200B" },
		});

		expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
	});

	it("closes the editor and discards the draft when the project changes", async () => {
		mockInstructions(MockChatProjectInstructions);
		const { user, rerenderWithProject } = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Edit" }));
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			" Draft for the first project.",
		);
		rerenderWithProject({
			...MockChatProject,
			id: "00000000-0000-4000-8000-0000000000b2",
		});

		// The open editor would hide the panel's Edit button from queries.
		await user.click(await screen.findByRole("button", { name: "Edit" }));
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

		await user.click(await screen.findByRole("button", { name: "Edit" }));
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

		await user.click(await screen.findByRole("button", { name: "Edit" }));
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
		// Two clicks in one task, before the pending state can render.
		const saveButton = screen.getByRole("button", { name: "Save" });
		fireEvent.click(saveButton);
		fireEvent.click(saveButton);

		await waitFor(() => expect(updateInstructions).toHaveBeenCalled());
		expect(updateInstructions).toHaveBeenCalledTimes(1);
	});

	it("refetches the instructions when the window regains focus", async () => {
		const getInstructions = mockInstructions(MockChatProjectInstructions);
		const { user } = renderPanel();
		await screen.findByRole("button", { name: "Edit" });

		// Another editor changed the instructions while this tab was hidden.
		getInstructions.mockResolvedValue({
			...MockChatProjectInstructions,
			instructions: "Edited in another tab.",
		});
		act(() => {
			focusManager.setFocused(false);
			focusManager.setFocused(true);
		});
		await waitFor(() => expect(getInstructions).toHaveBeenCalledTimes(2));

		await user.click(await screen.findByRole("button", { name: "Edit" }));
		await waitFor(() =>
			expect(screen.getByRole("textbox", { name: "Instructions" })).toHaveValue(
				"Edited in another tab.",
			),
		);
	});

	it("keeps Save disabled when a refetch changes the instructions under an unedited draft", async () => {
		const getInstructions = mockInstructions(MockChatProjectInstructions);
		const { user, queryClient } = renderPanel();
		await user.click(await screen.findByRole("button", { name: "Edit" }));

		getInstructions.mockResolvedValue({
			...MockChatProjectInstructions,
			instructions: "Edited in another tab.",
		});
		act(() => {
			focusManager.setFocused(false);
			focusManager.setFocused(true);
		});
		await waitFor(() =>
			expect(
				queryClient.getQueryData(
					chatProjectInstructionsKey(MockChatProject.id),
				),
			).toMatchObject({ instructions: "Edited in another tab." }),
		);

		expect(screen.getByRole("textbox", { name: "Instructions" })).toHaveValue(
			MockChatProjectInstructions.instructions,
		);
		expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
	});

	it("retries loading the instructions after an error", async () => {
		const getInstructions = vi
			.spyOn(API.experimental, "getChatProjectInstructions")
			.mockRejectedValueOnce(mockApiError({ message: "Failed to load." }))
			.mockResolvedValue(MockChatProjectInstructions);
		const { user } = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Retry" }));

		await waitFor(() => expect(getInstructions).toHaveBeenCalledTimes(2));
		await screen.findByRole("button", { name: "Edit" });
	});
});
