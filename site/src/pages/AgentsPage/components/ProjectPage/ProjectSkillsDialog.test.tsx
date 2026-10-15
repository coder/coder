import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Skill } from "#/api/typesGenerated";
import { MockChatProject } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { MockSkills } from "#/testHelpers/skills";
import { ProjectSkillsDialog } from "./ProjectSkillsDialog";

const { organization_id: orgId, id: projectId } = MockChatProject;

const mockReviewerSkill: Skill = {
	...MockSkills[0],
	content: "---\nname: reviewer\n---\nReview the diff.\n",
};

const mockConflict = {
	isAxiosError: true,
	response: {
		status: 409,
		data: { message: "A skill with that name already exists." },
	},
};

const renderDialog = ({ canEdit = true } = {}) => {
	const onOpenChange = vi.fn();
	const getSkills = vi
		.spyOn(API.experimental, "getChatProjectSkills")
		.mockResolvedValue([mockReviewerSkill]);
	const getSkill = vi
		.spyOn(API.experimental, "getChatProjectSkill")
		.mockResolvedValue(mockReviewerSkill);
	render(
		<QueryClientProvider client={createTestQueryClient()}>
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

	it("refreshes the skill list when the new name was just taken", async () => {
		vi.spyOn(API.experimental, "createChatProjectSkill").mockRejectedValue(
			mockConflict,
		);
		const { user, getSkills } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "New" }));
		await user.type(screen.getByRole("textbox", { name: "Name" }), "planner");
		await user.type(
			screen.getByRole("textbox", { name: "Body" }),
			"Plan the work.",
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(getSkills).toHaveBeenCalledTimes(2));
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

	it("opens skills for viewing without update access", async () => {
		const { user, getSkill } = renderDialog({ canEdit: false });

		await openSkillMenu(user);
		await user.click(screen.getByRole("menuitem", { name: "View" }));

		await waitFor(() =>
			expect(getSkill).toHaveBeenCalledWith(orgId, projectId, "reviewer"),
		);
	});
});
