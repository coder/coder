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
import type { ChatProjectInstructions } from "#/api/typesGenerated";
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

const renderPanel = () => {
	render(
		<ThemeOverride theme={themes[DEFAULT_THEME]}>
			<QueryClientProvider client={createTestQueryClient()}>
				<ProjectDetailsPanel project={MockChatProject} />
			</QueryClientProvider>
		</ThemeOverride>,
	);
	return userEvent.setup();
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
		const user = renderPanel();

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
		// The PUT response fills the cache, so the preview, the footer (which
		// falls back to the username of a user without a name), and the
		// reopened editor all reflect the saved instructions.
		expect(await screen.findByText(text)).toBeInTheDocument();
		expect(
			screen.getByText("Instructions updated by TestUser"),
		).toBeInTheDocument();
		await user.click(screen.getByRole("button", { name: "Edit" }));
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
		const user = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Edit" }));
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		expect(textbox).toHaveValue(MockChatProjectInstructions.instructions);
		const saveButton = screen.getByRole("button", { name: "Save" });
		expect(saveButton).toBeDisabled();
		// Surrounding whitespace alone is not a change worth saving.
		await user.type(textbox, "  ");
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

	it("deletes the instructions and returns to the empty state", async () => {
		const deleteInstructions = vi
			.spyOn(API.experimental, "deleteChatProjectInstructions")
			.mockResolvedValue();
		mockInstructions(MockChatProjectInstructions);
		const user = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Edit" }));
		await user.click(screen.getByRole("button", { name: "Delete" }));

		expect(deleteInstructions).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
		);
		await screen.findByRole("button", { name: "Create" });
		expect(screen.queryByText(/Instructions updated/)).not.toBeInTheDocument();
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
		const user = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Create" }));
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		await user.type(textbox, "Too long");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await screen.findByText("Instructions exceed maximum length.");
		// The detail names the limit the user has to stay under.
		expect(
			screen.getByText("Maximum length is 131072 bytes, got 140000."),
		).toBeInTheDocument();
		expect(textbox).toHaveValue("Too long");

		// Editing the draft dismisses the error about the old text.
		await user.type(textbox, "!");
		expect(
			screen.queryByText("Instructions exceed maximum length."),
		).not.toBeInTheDocument();
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
		const user = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Edit" }));
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			" More.",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));
		await screen.findByText("Failed to save.");
		await user.click(screen.getByRole("button", { name: "Delete" }));

		await screen.findByText("Failed to delete.");
		expect(screen.queryByText("Failed to save.")).not.toBeInTheDocument();
	});

	it("sends one update when Save is double clicked", async () => {
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockReturnValue(new Promise(() => {}));
		mockInstructions(MockUnsetChatProjectInstructions);
		const user = renderPanel();

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
		const user = renderPanel();
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

	it("retries loading the instructions after an error", async () => {
		const getInstructions = vi
			.spyOn(API.experimental, "getChatProjectInstructions")
			.mockRejectedValueOnce(mockApiError({ message: "Failed to load." }))
			.mockResolvedValue(MockChatProjectInstructions);
		const user = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Retry" }));

		await waitFor(() => expect(getInstructions).toHaveBeenCalledTimes(2));
		await screen.findByRole("button", { name: "Edit" });
	});
});
