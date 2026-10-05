import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
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
		// The PUT response fills the cache, so reopening edits the saved text.
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
		const user = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Edit" }));
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		expect(textbox).toHaveValue(MockChatProjectInstructions.instructions);
		const saveButton = screen.getByRole("button", { name: "Save" });
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
	});

	it("keeps the editor open with the draft when saving fails", async () => {
		vi.spyOn(
			API.experimental,
			"updateChatProjectInstructions",
		).mockRejectedValue(
			mockApiError({ message: "Instructions exceed maximum length." }),
		);
		mockInstructions(MockUnsetChatProjectInstructions);
		const user = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Create" }));
		const textbox = screen.getByRole("textbox", { name: "Instructions" });
		await user.type(textbox, "Too long");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await screen.findByText("Instructions exceed maximum length.");
		expect(textbox).toHaveValue("Too long");
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
