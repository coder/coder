import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { focusManager, type QueryClient } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatProjectInstructionsKey } from "#/api/queries/chatProjects";
import type {
	ChatProject,
	ChatProjectInstructions,
} from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectInstructions,
	MockUnsetChatProjectInstructions,
	mockApiError,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { ProjectInstructionsSection } from "./ProjectInstructionsSection";

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

const confirmDelete = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("button", { name: "Delete" }));
	await user.click(
		await screen.findByRole("button", { name: "Delete instructions" }),
	);
};

const findTextbox = () =>
	screen.findByRole("textbox", { name: "Instructions" });

const renderSection = (project: ChatProject = MockChatProject) => {
	const result = render(<ProjectInstructionsSection project={project} />);
	return { user: userEvent.setup(), queryClient: result.queryClient };
};

describe("ProjectInstructionsSection", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		focusManager.setFocused(undefined);
	});

	it("saves new instructions and keeps them without refetching", async () => {
		const text = "Use TypeScript for new code.";
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockResolvedValue({
				...MockChatProjectInstructions,
				instructions: text,
			});
		const getInstructions = mockInstructions(MockUnsetChatProjectInstructions);
		const { user, queryClient } = renderSection();

		await user.type(await findTextbox(), text);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(updateInstructions).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
			{ instructions: text },
		);
		await waitFor(() =>
			expect(
				queryClient.getQueryData(
					chatProjectInstructionsKey(MockChatProject.id),
				),
			).toEqual({ ...MockChatProjectInstructions, instructions: text }),
		);
		expect(await findTextbox()).toHaveValue(text);
		expect(getInstructions).toHaveBeenCalledTimes(1);
	});

	it("discards an edit", async () => {
		mockInstructions(MockChatProjectInstructions);
		const { user } = renderSection();

		await user.type(await findTextbox(), " Draft.");
		await user.click(screen.getByRole("button", { name: "Discard" }));

		expect(await findTextbox()).toHaveValue(
			MockChatProjectInstructions.instructions,
		);
	});

	it("deletes the instructions after clearing the text", async () => {
		const deleteInstructions = vi
			.spyOn(API.experimental, "deleteChatProjectInstructions")
			.mockResolvedValue();
		mockInstructions(MockChatProjectInstructions);
		const { user, queryClient } = renderSection();

		await user.clear(await findTextbox());
		await confirmDelete(user);

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

	it("keeps the draft when saving fails", async () => {
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockRejectedValue(
				mockApiError({ message: "Instructions exceed maximum length." }),
			);
		mockInstructions(MockUnsetChatProjectInstructions);
		const { user } = renderSection();

		const textbox = await findTextbox();
		await user.type(textbox, "Too long");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(updateInstructions).toHaveBeenCalledTimes(1));
		await waitFor(() => expect(textbox).not.toHaveAttribute("readonly"));
		expect(textbox).toHaveValue("Too long");
	});

	it("sends one update when Save is double clicked", async () => {
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockReturnValue(new Promise(() => {}));
		mockInstructions(MockUnsetChatProjectInstructions);
		const { user } = renderSection();

		await user.type(await findTextbox(), "Be brief.");
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
		const { queryClient } = renderSection();
		await findTextbox();

		// Another editor changed the instructions while this tab was hidden.
		await refetchWith(getInstructions, queryClient, "Edited in another tab.");

		await waitFor(async () =>
			expect(await findTextbox()).toHaveValue("Edited in another tab."),
		);
	});

	it("blocks Save until the user resolves a refetch that changed the instructions", async () => {
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockResolvedValue(MockChatProjectInstructions);
		const getInstructions = mockInstructions(MockChatProjectInstructions);
		const { user, queryClient } = renderSection();
		const textbox = await findTextbox();
		await user.type(textbox, " My edit.");

		// Another editor changed the instructions while this one was open.
		await refetchWith(getInstructions, queryClient, "Edited in another tab.");

		await user.type(textbox, " More.");
		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(updateInstructions).not.toHaveBeenCalled();

		await user.click(screen.getByRole("button", { name: "Keep my draft" }));
		await user.click(screen.getByRole("button", { name: "Save" }));
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
			text: "Saved text.",
			submit: (user: ReturnType<typeof userEvent.setup>) =>
				user.click(screen.getByRole("button", { name: "Save" })),
		},
		{
			action: "Delete",
			result: MockUnsetChatProjectInstructions,
			text: "",
			submit: confirmDelete,
		},
	])(
		"keeps the result of $action when a stale refetch finishes after it",
		async ({ result, text, submit }) => {
			vi.spyOn(
				API.experimental,
				"updateChatProjectInstructions",
			).mockResolvedValue(result);
			vi.spyOn(
				API.experimental,
				"deleteChatProjectInstructions",
			).mockResolvedValue();
			const getInstructions = mockInstructions(MockChatProjectInstructions);
			const { user, queryClient } = renderSection();
			const textbox = await findTextbox();
			await user.clear(textbox);
			if (text !== "") {
				await user.type(textbox, text);
			}

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
			await submit(user);
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
		const { user } = renderSection();

		await user.click(await screen.findByRole("button", { name: "Retry" }));

		await waitFor(() => expect(getInstructions).toHaveBeenCalledTimes(2));
	});
});
