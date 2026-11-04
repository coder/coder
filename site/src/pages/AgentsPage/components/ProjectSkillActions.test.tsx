import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { focusManager, QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { SkillMetadata } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { MockChatProject } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { ProjectSkillActions } from "./ProjectSkillActions";

// "audit" and "check" sort first, so either would take an inline slot if
// non-action or disabled skills were offered.
const mockSkills: SkillMetadata[] = [
	{ ...MockSkill, id: "skill-triage", name: "triage", action: true },
	{ ...MockSkill, id: "skill-audit", name: "audit", action: false },
	{ ...MockSkill, id: "skill-review", name: "review", action: true },
	{
		...MockSkill,
		id: "skill-check",
		name: "check",
		action: true,
		enabled: false,
	},
	{ ...MockSkill, id: "skill-release", name: "release", action: true },
	{ ...MockSkill, id: "skill-lint", name: "lint", action: true },
	{ ...MockSkill, id: "skill-deploy", name: "deploy", action: true },
];

// "Assess issue" sorts first and "Wrap up" last, unlike their names.
const mockSkillsWithDisplayNames: SkillMetadata[] = [
	{ ...mockSkills[0], display_name: "Assess issue" },
	mockSkills[1],
	{ ...mockSkills[2], display_name: "Wrap up" },
	...mockSkills.slice(3),
];

const renderActions = (skills: SkillMetadata[] = mockSkills) => {
	vi.spyOn(API.experimental, "getChatProjectSkills").mockResolvedValue(skills);
	const onInvoke = vi.fn();
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<TooltipProvider>
				<ProjectSkillActions
					organizationId={MockChatProject.organization_id}
					projectId={MockChatProject.id}
					onInvoke={onInvoke}
				/>
			</TooltipProvider>
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), onInvoke };
};

describe("ProjectSkillActions", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		focusManager.setFocused(undefined);
	});

	it("offers the first three actions by name and the rest under More", async () => {
		const { user, onInvoke } = renderActions();

		await user.click(await screen.findByRole("button", { name: "deploy" }));
		await user.click(screen.getByRole("button", { name: "lint" }));
		await user.click(screen.getByRole("button", { name: "release" }));
		await user.click(
			screen.getByRole("button", { name: "More project actions" }),
		);
		await user.click(await screen.findByRole("menuitem", { name: "review" }));
		await waitFor(() => expect(onInvoke).toHaveBeenLastCalledWith("review"));
		await user.click(
			screen.getByRole("button", { name: "More project actions" }),
		);
		await user.click(await screen.findByRole("menuitem", { name: "triage" }));

		await waitFor(() =>
			expect(onInvoke.mock.calls).toEqual([
				["deploy"],
				["lint"],
				["release"],
				["review"],
				["triage"],
			]),
		);
	});

	it("labels and sorts actions by display name but invokes them by name", async () => {
		const { user, onInvoke } = renderActions(mockSkillsWithDisplayNames);

		// "Assess issue" now sorts first, which moves "release" under More.
		await user.click(
			await screen.findByRole("button", { name: "Assess issue" }),
		);
		await user.click(screen.getByRole("button", { name: "lint" }));
		await user.click(
			screen.getByRole("button", { name: "More project actions" }),
		);
		await user.click(await screen.findByRole("menuitem", { name: "release" }));
		await waitFor(() => expect(onInvoke).toHaveBeenLastCalledWith("release"));
		await user.click(
			screen.getByRole("button", { name: "More project actions" }),
		);
		await user.click(await screen.findByRole("menuitem", { name: "Wrap up" }));

		await waitFor(() =>
			expect(onInvoke.mock.calls).toEqual([
				["triage"],
				["lint"],
				["release"],
				["review"],
			]),
		);
	});

	it("refetches the actions when the window regains focus", async () => {
		renderActions();
		await screen.findByRole("button", { name: "deploy" });
		const getSkills = vi.mocked(API.experimental.getChatProjectSkills);
		expect(getSkills).toHaveBeenCalledTimes(1);

		// Another tab may have changed the project's actions.
		act(() => {
			focusManager.setFocused(false);
			focusManager.setFocused(true);
		});

		await waitFor(() => expect(getSkills).toHaveBeenCalledTimes(2));
	});
});
