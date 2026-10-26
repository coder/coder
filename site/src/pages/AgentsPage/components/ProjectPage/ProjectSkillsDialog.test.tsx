import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { saveAs } from "file-saver";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatProjectSkillKey } from "#/api/queries/chatProjectSkills";
import type { Skill, SkillMetadata } from "#/api/typesGenerated";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { MockSkills } from "#/testHelpers/skills";
import { parseSkillMarkdown } from "../../utils/skills";
import { ProjectSkillsDialog } from "./ProjectSkillsDialog";

vi.mock("file-saver", () => ({ saveAs: vi.fn() }));

const { organization_id: orgId, id: projectId } = MockChatProject;

const mockReviewerSkill: Skill = {
	...MockSkills[0],
	content: "---\nname: reviewer\n---\nReview the diff.\n",
};

type RenderDialogOptions = {
	canEdit?: boolean;
	queryClient?: QueryClient;
	skills?: SkillMetadata[];
};

const renderDialog = ({
	canEdit = true,
	queryClient = createTestQueryClient(),
	skills = [mockReviewerSkill],
}: RenderDialogOptions = {}) => {
	const onOpenChange = vi.fn();
	const getSkills = vi
		.spyOn(API.experimental, "getChatProjectSkills")
		.mockResolvedValue(skills);
	const getSkill = vi
		.spyOn(API.experimental, "getChatProjectSkill")
		.mockResolvedValue(mockReviewerSkill);
	render(
		<QueryClientProvider client={queryClient}>
			<ProjectSkillsDialog
				project={MockChatProject}
				open
				onOpenChange={onOpenChange}
				canEdit={canEdit}
				onCloseAutoFocus={vi.fn()}
			/>
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), getSkills, getSkill, onOpenChange };
};

const openSkillMenu = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(
		await screen.findByRole("button", { name: "Open menu for reviewer" }),
	);
};

describe("ProjectSkillsDialog", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("creates a skill and refreshes the project's skill list", async () => {
		const createSkill = vi
			.spyOn(API.experimental, "createChatProjectSkill")
			.mockResolvedValue({ ...mockReviewerSkill, name: "planner" });
		const { user, getSkills } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "New" }));
		await user.type(screen.getByRole("textbox", { name: "Name" }), "planner");
		await user.type(
			screen.getByRole("textbox", { name: "Body" }),
			"Plan the work.",
		);
		await user.click(screen.getByRole("switch", { name: "Show as action" }));
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(createSkill).toHaveBeenCalledWith(orgId, projectId, {
				content: "---\nname: planner\n---\nPlan the work.\n",
				action: true,
				model_invocable: true,
			}),
		);
		await waitFor(() => expect(getSkills).toHaveBeenCalledTimes(2));
	});

	it("sends one create request when Save is double-clicked", async () => {
		const createSkill = vi
			.spyOn(API.experimental, "createChatProjectSkill")
			.mockReturnValue(new Promise(() => {}));
		const { user } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "New" }));
		await user.type(screen.getByRole("textbox", { name: "Name" }), "planner");
		await user.type(
			screen.getByRole("textbox", { name: "Body" }),
			"Plan the work.",
		);
		await user.dblClick(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(createSkill).toHaveBeenCalled());
		expect(createSkill).toHaveBeenCalledTimes(1);
	});

	it("refreshes the list after a name conflict and lets Save retry", async () => {
		const conflict = mockApiError({
			message: "A skill with that name already exists.",
		});
		const createSkill = vi
			.spyOn(API.experimental, "createChatProjectSkill")
			.mockRejectedValue({
				...conflict,
				response: { ...conflict.response, status: 409 },
			});
		const { user, getSkills } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "New" }));
		await user.type(screen.getByRole("textbox", { name: "Name" }), "planner");
		await user.type(
			screen.getByRole("textbox", { name: "Body" }),
			"Plan the work.",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(getSkills).toHaveBeenCalledTimes(2));
		await user.click(screen.getByRole("button", { name: "Save" }));
		await waitFor(() => expect(createSkill).toHaveBeenCalledTimes(2));
	});

	it("fills a new skill and its automatic use from pasted SKILL.md content", async () => {
		const createSkill = vi
			.spyOn(API.experimental, "createChatProjectSkill")
			.mockResolvedValue({ ...mockReviewerSkill, name: "planner" });
		const { user } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "New" }));
		await user.click(
			screen.getByRole("textbox", { name: "Import from SKILL.md" }),
		);
		await user.paste(
			"---\nname: planner\ndescription: Plans work.\ndisable-model-invocation: true\n---\nPlan the work.",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(createSkill).toHaveBeenCalledWith(orgId, projectId, {
				content:
					'---\nname: planner\ndescription: "Plans work."\n---\nPlan the work.\n',
				action: false,
				model_invocable: false,
			}),
		);
	});

	it("resets automatic use when a later SKILL.md import omits it", async () => {
		const createSkill = vi
			.spyOn(API.experimental, "createChatProjectSkill")
			.mockResolvedValue({ ...mockReviewerSkill, name: "planner" });
		const { user } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "New" }));
		const importField = screen.getByRole("textbox", {
			name: "Import from SKILL.md",
		});
		await user.click(importField);
		await user.paste(
			"---\nname: drafter\ndisable-model-invocation: true\n---\nDraft it.",
		);
		await user.click(importField);
		await user.paste("---\nname: planner\n---\nPlan the work.");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(createSkill).toHaveBeenCalledWith(
				orgId,
				projectId,
				expect.objectContaining({ model_invocable: true }),
			),
		);
	});

	it("edits a skill's body and keeps its name", async () => {
		const updateSkill = vi
			.spyOn(API.experimental, "updateChatProjectSkill")
			.mockResolvedValue(mockReviewerSkill);
		const { user } = renderDialog();

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Edit" }));
		const body = await screen.findByRole("textbox", { name: "Body" });
		await user.clear(body);
		await user.type(body, "Review carefully.");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(updateSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer", {
				content:
					'---\nname: reviewer\ndescription: "Review changed files and suggest fixes."\n---\nReview carefully.\n',
				action: false,
				model_invocable: true,
			}),
		);
	});

	it("saves a change to only a skill's automatic use without rewriting SKILL.md", async () => {
		const updateSkill = vi
			.spyOn(API.experimental, "updateChatProjectSkill")
			.mockResolvedValue(mockReviewerSkill);
		const { user } = renderDialog();

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Edit" }));
		await user.click(
			await screen.findByRole("switch", {
				name: "Agents can use it automatically",
			}),
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(updateSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer", {
				content: undefined,
				action: false,
				model_invocable: false,
			}),
		);
	});

	it("keeps edits made over cached content when a refetch differs", async () => {
		// Unlike the test client, keep the seeded detail cached while unobserved.
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		queryClient.setQueryData(
			chatProjectSkillKey(projectId, "reviewer"),
			mockReviewerSkill,
		);
		const updateSkill = vi
			.spyOn(API.experimental, "updateChatProjectSkill")
			.mockResolvedValue(mockReviewerSkill);
		const { user, getSkill } = renderDialog({ queryClient });
		let resolveRefetch: (skill: Skill) => void = () => {};
		getSkill.mockReturnValue(
			new Promise((resolve) => {
				resolveRefetch = resolve;
			}),
		);

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Edit" }));
		const body = await screen.findByRole("textbox", { name: "Body" });
		await user.clear(body);
		await user.type(body, "Review carefully.");
		await waitFor(() => expect(getSkill).toHaveBeenCalled());
		await act(async () => {
			resolveRefetch({
				...mockReviewerSkill,
				content: "---\nname: reviewer\n---\nRefreshed elsewhere.\n",
			});
		});
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(updateSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer", {
				content: expect.stringContaining("Review carefully."),
				action: false,
				model_invocable: true,
			}),
		);
	});

	it("returns to a refreshed list when the edited skill was deleted", async () => {
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		queryClient.setQueryData(
			chatProjectSkillKey(projectId, "reviewer"),
			mockReviewerSkill,
		);
		const { user, getSkill, getSkills, onOpenChange } = renderDialog({
			queryClient,
		});
		const notFound = mockApiError({ message: "Skill not found." });
		getSkill.mockRejectedValue({
			...notFound,
			response: { ...notFound.response, status: 404 },
		});

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Edit" }));

		await waitFor(() => expect(getSkills).toHaveBeenCalledTimes(2));
		// Escape closes the dialog from the list, while the editor keeps it open.
		await user.keyboard("{Escape}");
		expect(onOpenChange).toHaveBeenCalledWith(false);
	});

	it("asks before Escape discards unsaved edits", async () => {
		const updateSkill = vi
			.spyOn(API.experimental, "updateChatProjectSkill")
			.mockResolvedValue(mockReviewerSkill);
		const { user, onOpenChange } = renderDialog();

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Edit" }));
		const body = await screen.findByRole("textbox", { name: "Body" });
		await user.clear(body);
		await user.type(body, "Review carefully.");
		await user.keyboard("{Escape}");
		await user.click(
			await screen.findByRole("button", { name: "Keep editing" }),
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onOpenChange).not.toHaveBeenCalled();
		await waitFor(() =>
			expect(updateSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer", {
				content:
					'---\nname: reviewer\ndescription: "Review changed files and suggest fixes."\n---\nReview carefully.\n',
				action: false,
				model_invocable: true,
			}),
		);
	});

	it("turns a skill off from the list", async () => {
		const updateSkill = vi
			.spyOn(API.experimental, "updateChatProjectSkill")
			.mockResolvedValue({ ...mockReviewerSkill, enabled: false });
		const { user } = renderDialog();

		await user.click(
			await screen.findByRole("switch", { name: "Enable reviewer" }),
		);

		await waitFor(() =>
			expect(updateSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer", {
				enabled: false,
			}),
		);
	});

	it("deletes a skill after confirmation", async () => {
		const deleteSkill = vi
			.spyOn(API.experimental, "deleteChatProjectSkill")
			.mockResolvedValue();
		const { user, getSkills } = renderDialog();

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: /Delete/ }));
		await user.click(
			await screen.findByRole("button", { name: "Delete skill" }),
		);

		await waitFor(() =>
			expect(deleteSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer"),
		);
		await waitFor(() => expect(getSkills).toHaveBeenCalledTimes(2));
	});

	it("drops a skill's cached details when its delete finds it already gone", async () => {
		// Unlike the test client, keep the seeded detail cached while unobserved.
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		const detailKey = chatProjectSkillKey(projectId, "reviewer");
		queryClient.setQueryData(detailKey, mockReviewerSkill);
		const notFound = mockApiError({ message: "Skill not found." });
		vi.spyOn(API.experimental, "deleteChatProjectSkill").mockRejectedValue({
			...notFound,
			response: { ...notFound.response, status: 404 },
		});
		const { user, getSkills } = renderDialog({ queryClient });

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: /Delete/ }));
		await user.click(
			await screen.findByRole("button", { name: "Delete skill" }),
		);

		await waitFor(() => expect(getSkills).toHaveBeenCalledTimes(2));
		expect(queryClient.getQueryData(detailKey)).toBeUndefined();
	});

	it("refreshes the list when a downloaded skill was deleted", async () => {
		const { user, getSkill, getSkills } = renderDialog();
		const notFound = mockApiError({ message: "Skill not found." });
		getSkill.mockRejectedValue({
			...notFound,
			response: { ...notFound.response, status: 404 },
		});

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Download" }));

		await waitFor(() => expect(getSkills).toHaveBeenCalledTimes(2));
		expect(saveAs).not.toHaveBeenCalled();
	});

	it("keeps the saved content when an older detail fetch finishes later", async () => {
		// Unlike the test client, keep the seeded detail cached while unobserved.
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		const detailKey = chatProjectSkillKey(projectId, "reviewer");
		queryClient.setQueryData(detailKey, mockReviewerSkill);
		const mockSavedSkill = {
			...mockReviewerSkill,
			content: "---\nname: reviewer\n---\nSaved.\n",
		};
		vi.spyOn(API.experimental, "updateChatProjectSkill").mockResolvedValue(
			mockSavedSkill,
		);
		const { user, getSkill } = renderDialog({ queryClient });
		let resolveOlderFetch: (skill: Skill) => void = () => {};
		getSkill.mockReturnValue(
			new Promise((resolve) => {
				resolveOlderFetch = resolve;
			}),
		);

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Edit" }));
		const body = await screen.findByRole("textbox", { name: "Body" });
		await waitFor(() => expect(getSkill).toHaveBeenCalled());
		await user.clear(body);
		await user.type(body, "Saved.");
		await user.click(screen.getByRole("button", { name: "Save" }));
		await waitFor(() =>
			expect(queryClient.getQueryData(detailKey)).toEqual(mockSavedSkill),
		);
		await act(async () => resolveOlderFetch(mockReviewerSkill));

		expect(queryClient.getQueryData(detailKey)).toEqual(mockSavedSkill);
	});

	it("keeps New from opening the editor at the project skill cap", async () => {
		const mockSkillsAtCapacity = Array.from({ length: 100 }, (_, index) => ({
			...MockSkills[0],
			id: `skill-${index}`,
			name: `skill-${index}`,
		}));
		const { user, onOpenChange } = renderDialog({
			skills: mockSkillsAtCapacity,
		});

		await screen.findByRole("button", { name: "Open menu for skill-0" });
		await user.click(screen.getByRole("button", { name: "New" }));
		// Escape closes the dialog from the list, while the editor keeps it open.
		await user.keyboard("{Escape}");
		expect(onOpenChange).toHaveBeenCalledWith(false);
	});

	it("downloads a skill's SKILL.md content", async () => {
		const { user, getSkill } = renderDialog();

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Download" }));

		await waitFor(() => expect(saveAs).toHaveBeenCalledTimes(1));
		expect(getSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer");
		const [file, fileName] = vi.mocked(saveAs).mock.calls[0];
		if (!(file instanceof Blob)) {
			throw new Error("Expected the download to be a Blob.");
		}
		expect(fileName).toBe("reviewer.md");
		expect(await file.text()).toBe(mockReviewerSkill.content);
	});

	it("marks a downloaded manual-only skill so re-importing keeps it manual", async () => {
		const { user, getSkill } = renderDialog();
		getSkill.mockResolvedValue({
			...mockReviewerSkill,
			model_invocable: false,
		});

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Download" }));

		await waitFor(() => expect(saveAs).toHaveBeenCalledTimes(1));
		const [file] = vi.mocked(saveAs).mock.calls[0];
		if (!(file instanceof Blob)) {
			throw new Error("Expected the download to be a Blob.");
		}
		expect(parseSkillMarkdown(await file.text()).modelInvocable).toBe(false);
	});

	it("opens skills for viewing without update access", async () => {
		const { user, getSkill, onOpenChange } = renderDialog({ canEdit: false });

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "View" }));

		await waitFor(() =>
			expect(getSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer"),
		);
		await user.keyboard("{Escape}");
		expect(onOpenChange).toHaveBeenCalledWith(false);
	});
});
