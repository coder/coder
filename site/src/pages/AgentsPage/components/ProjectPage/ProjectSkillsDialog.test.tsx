import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { saveAs } from "file-saver";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatProjectSkill } from "#/api/queries/chatProjectSkills";
import type { Skill } from "#/api/typesGenerated";
import { MockChatProject, mockApiError } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { MockSkills } from "#/testHelpers/skills";
import { ProjectSkillsDialog } from "./ProjectSkillsDialog";

vi.mock("file-saver", () => ({ saveAs: vi.fn() }));

const { organization_id: orgId, id: projectId } = MockChatProject;

const mockReviewerSkill: Skill = {
	...MockSkills[0],
	content: "---\nname: reviewer\n---\nReview the diff.\n",
};

const renderDialog = ({
	canEdit = true,
	queryClient = createTestQueryClient(),
} = {}) => {
	const onOpenChange = vi.fn();
	const getSkills = vi
		.spyOn(API.experimental, "getChatProjectSkills")
		.mockResolvedValue([mockReviewerSkill]);
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
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(createSkill).toHaveBeenCalledWith(orgId, projectId, {
				content: "---\nname: planner\n---\nPlan the work.\n",
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
		await screen.findByText("A skill with that name already exists.");
		await user.click(screen.getByRole("button", { name: "Save" }));
		await waitFor(() => expect(createSkill).toHaveBeenCalledTimes(2));
	});

	it("fills a new skill from pasted SKILL.md content", async () => {
		const createSkill = vi
			.spyOn(API.experimental, "createChatProjectSkill")
			.mockResolvedValue({ ...mockReviewerSkill, name: "planner" });
		const { user } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "New" }));
		await user.click(
			screen.getByRole("textbox", { name: "Import from SKILL.md" }),
		);
		await user.paste(
			"---\nname: planner\ndescription: Plans work.\n---\nPlan the work.",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() =>
			expect(createSkill).toHaveBeenCalledWith(orgId, projectId, {
				content:
					'---\nname: planner\ndescription: "Plans work."\n---\nPlan the work.\n',
			}),
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
			}),
		);
	});

	it("keeps edits made over cached content when a refetch differs", async () => {
		// Unlike the test client, keep the seeded detail cached while unobserved.
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		queryClient.setQueryData(
			chatProjectSkill(orgId, projectId, "reviewer").queryKey,
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
			}),
		);
	});

	it("returns to a refreshed list when the edited skill was deleted", async () => {
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		queryClient.setQueryData(
			chatProjectSkill(orgId, projectId, "reviewer").queryKey,
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
		const detailKey = chatProjectSkill(orgId, projectId, "reviewer").queryKey;
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

	it("downloads a skill's SKILL.md content", async () => {
		const { user, getSkill } = renderDialog();

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "Download" }));

		await waitFor(() => expect(saveAs).toHaveBeenCalledTimes(1));
		expect(getSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer");
		const [file, fileName] = vi.mocked(saveAs).mock.calls[0];
		expect(fileName).toBe("reviewer.md");
		expect(await (file as Blob).text()).toBe(mockReviewerSkill.content);
	});

	it("opens skills for viewing without update access", async () => {
		const { user, getSkill, onOpenChange } = renderDialog({ canEdit: false });

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "View" }));

		await waitFor(() =>
			expect(getSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer"),
		);
		await screen.findByRole("textbox", { name: "Body" });
		await user.keyboard("{Escape}");
		expect(onOpenChange).toHaveBeenCalledWith(false);
	});
});
