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
import {
	richParametersKey,
	templateVersionPresetsKey,
} from "#/api/queries/templates";
import type {
	ChatProject,
	ChatProjectInstructions,
	Preset,
	Template,
} from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import {
	MockChatProject,
	MockChatProjectInstructions,
	MockTemplate,
	MockTemplateVersionParameter1,
	MockUnsetChatProjectInstructions,
	mockApiError,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { ProjectDetailsPanel } from "./ProjectDetailsPanel";

vi.mock("#/hooks/useAuthenticated", async () => {
	const { MockUserOwner } = await import("#/testHelpers/entities");
	return {
		useAuthenticated: () => ({ user: MockUserOwner, permissions: {} }),
	};
});

vi.mock("#/modules/dashboard/useDashboard", async () => {
	const { MockDefaultOrganization } = await import("#/testHelpers/entities");
	return {
		useDashboard: () => ({
			organizations: [MockDefaultOrganization],
			showOrganizations: true,
		}),
	};
});

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

/** Opens the delete confirmation and confirms it. */
const confirmDelete = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("button", { name: "Delete" }));
	await user.click(
		await screen.findByRole("button", { name: "Delete instructions" }),
	);
};

const renderPanel = () => {
	const queryClient = createTestQueryClient();
	const panel = (project: ChatProject) => (
		<ThemeOverride theme={themes[DEFAULT_THEME]}>
			<QueryClientProvider client={queryClient}>
				<TooltipProvider>
					<ProjectDetailsPanel project={project} />
				</TooltipProvider>
			</QueryClientProvider>
		</ThemeOverride>
	);
	const { rerender } = render(panel(MockChatProject));
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

		await user.click(
			await screen.findByRole("button", { name: "Create instructions" }),
		);
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

		await user.click(
			await screen.findByRole("button", { name: "Create instructions" }),
		);
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
		await confirmDelete(user);

		await waitFor(() => expect(dialog).toHaveTextContent("Failed to delete."));
		expect(dialog).not.toHaveTextContent("Failed to save.");
	});

	it("sends one update when Save is double clicked", async () => {
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockReturnValue(new Promise(() => {}));
		mockInstructions(MockUnsetChatProjectInstructions);
		const { user } = renderPanel();

		await user.click(
			await screen.findByRole("button", { name: "Create instructions" }),
		);
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

	it("closes the editor when a double clicked Save succeeds after a failure", async () => {
		const updateInstructions = vi
			.spyOn(API.experimental, "updateChatProjectInstructions")
			.mockRejectedValueOnce(mockApiError({ message: "Failed to save." }))
			.mockResolvedValue({
				...MockChatProjectInstructions,
				instructions: "Be brief.",
			});
		mockInstructions(MockUnsetChatProjectInstructions);
		const { user } = renderPanel();

		await user.click(
			await screen.findByRole("button", { name: "Create instructions" }),
		);
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			"Be brief.",
		);
		const dialog = screen.getByRole("dialog");
		await user.click(screen.getByRole("button", { name: "Save" }));
		await waitFor(() => expect(dialog).toHaveTextContent("Failed to save."));
		// The second click still sees the failed save's error, and must not
		// reset the retry that the first click started.
		await userEvent
			.setup({ delay: null })
			.dblClick(screen.getByRole("button", { name: "Save" }));

		// Reopening needs onSuccess to close the editor, and a third save
		// needs onSettled to clear the in-flight guard.
		await user.click(
			await screen.findByRole("button", { name: "Edit instructions" }),
		);
		await user.type(
			screen.getByRole("textbox", { name: "Instructions" }),
			" More.",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));
		await waitFor(() => expect(updateInstructions).toHaveBeenCalledTimes(3));
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
			submit: (user: ReturnType<typeof userEvent.setup>) =>
				user.click(screen.getByRole("button", { name: "Save" })),
		},
		{
			action: "Delete",
			result: MockUnsetChatProjectInstructions,
			submit: confirmDelete,
		},
	])(
		"keeps the result of $action when a stale refetch finishes after it",
		async ({ result, submit }) => {
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
		const { user } = renderPanel();

		await user.click(await screen.findByRole("button", { name: "Retry" }));

		await waitFor(() => expect(getInstructions).toHaveBeenCalledTimes(2));
		await screen.findByRole("button", { name: "Edit instructions" });
	});
});

const dockerTemplate: Template = {
	...MockTemplate,
	id: "template-docker",
	name: "docker",
	display_name: "Docker",
};
const kubernetesTemplate: Template = {
	...MockTemplate,
	id: "template-kubernetes",
	name: "kubernetes",
	display_name: "Kubernetes",
};

describe("ProjectDetailsPanel templates", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	const setup = () => {
		mockInstructions(MockUnsetChatProjectInstructions);
		vi.spyOn(API, "getTemplates").mockResolvedValue([
			dockerTemplate,
			kubernetesTemplate,
		]);
		const updateProject = vi
			.spyOn(API.experimental, "updateChatProject")
			.mockResolvedValue(MockChatProject);
		return { updateProject, ...renderPanel() };
	};

	it("saves the templates picked in the editor", async () => {
		const { user, updateProject } = setup();

		await user.click(
			screen.getByRole("button", { name: "Edit templates: 0 templates" }),
		);
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{
					templates: {
						template_ids: [dockerTemplate.id, kubernetesTemplate.id],
					},
				},
			),
		);
	});

	it("sends one templates update when Save is double clicked", async () => {
		const { user, updateProject } = setup();
		updateProject.mockReturnValue(new Promise(() => {}));

		await user.click(
			screen.getByRole("button", { name: "Edit templates: 0 templates" }),
		);
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		await userEvent
			.setup({ delay: null })
			.dblClick(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(updateProject).toHaveBeenCalled());
		expect(updateProject).toHaveBeenCalledTimes(1);
	});

	it("starts the editor from the saved templates", async () => {
		const { user, updateProject, rerenderWithProject } = setup();
		rerenderWithProject({
			...MockChatProject,
			templates: {
				template_ids: [dockerTemplate.id, kubernetesTemplate.id],
			},
		});

		await user.click(
			screen.getByRole("button", { name: "Edit templates: 2 templates" }),
		);
		await user.click(
			await screen.findByRole("combobox", { name: "Templates" }),
		);
		await user.keyboard("{Backspace}");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ templates: { template_ids: [dockerTemplate.id] } },
			),
		);
	});

	it("keeps the draft when the project refetches while editing", async () => {
		const { user, updateProject, rerenderWithProject } = setup();

		await user.click(
			screen.getByRole("button", { name: "Edit templates: 0 templates" }),
		);
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		rerenderWithProject({
			...MockChatProject,
			templates: { template_ids: [kubernetesTemplate.id] },
		});
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{
					templates: {
						template_ids: [dockerTemplate.id, kubernetesTemplate.id],
					},
				},
			),
		);
	});

	it.each([
		{
			kind: "presets",
			templates: { default_preset_id: "preset-small" },
			key: templateVersionPresetsKey(dockerTemplate.active_version_id),
			cached: [
				{
					ID: "preset-small",
					Name: "Small",
					Parameters: [],
					Default: false,
					DesiredPrebuildInstances: null,
					Description: "",
					Icon: "",
				},
			] satisfies Preset[],
			cachedLabel: "Default preset",
			mockFailure: () =>
				vi
					.spyOn(API, "getTemplateVersionPresets")
					.mockRejectedValue(
						mockApiError({ message: "Presets are unavailable." }),
					),
			message: "Presets are unavailable.",
		},
		{
			kind: "parameters",
			templates: {
				default_parameters: { [MockTemplateVersionParameter1.name]: "x" },
			},
			key: richParametersKey(dockerTemplate.active_version_id),
			cached: [MockTemplateVersionParameter1],
			cachedLabel: MockTemplateVersionParameter1.name,
			mockFailure: () =>
				vi
					.spyOn(API, "getTemplateVersionRichParameters")
					.mockRejectedValue(
						mockApiError({ message: "Parameters are unavailable." }),
					),
			message: "Parameters are unavailable.",
		},
	])(
		"shows a failed $kind refetch alongside the cached values",
		async ({ templates, key, cached, cachedLabel, mockFailure, message }) => {
			const { user, queryClient, rerenderWithProject } = setup();
			const fetchDefaults = mockFailure();
			// The test client drops unobserved queries at once, so keep the
			// cached values around until the editor mounts.
			queryClient.setQueryDefaults(key, { gcTime: Number.POSITIVE_INFINITY });
			queryClient.setQueryData(key, cached);
			rerenderWithProject({
				...MockChatProject,
				templates: {
					template_ids: [dockerTemplate.id],
					default_template_id: dockerTemplate.id,
					...templates,
				},
			});

			await user.click(
				screen.getByRole("button", { name: "Edit templates: 1 template" }),
			);

			await waitFor(() => expect(fetchDefaults).toHaveBeenCalled());
			expect(screen.getByLabelText(cachedLabel)).toBeInTheDocument();
			expect(await screen.findByRole("alert")).toHaveTextContent(message);
		},
	);
});
