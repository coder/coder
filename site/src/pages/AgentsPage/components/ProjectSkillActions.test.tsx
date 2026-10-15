import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import type { SkillMetadata } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { MockChatProject } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { ProjectSkillActions } from "./ProjectSkillActions";

const mockSkill = (
	name: string,
	overrides: Partial<SkillMetadata> = {},
): SkillMetadata => ({
	...MockSkill,
	id: `skill-${name}`,
	name,
	description: `Runs ${name}.`,
	action: true,
	...overrides,
});

const renderActions = (skills: SkillMetadata[]) => {
	vi.spyOn(API.experimental, "getChatProjectSkills").mockResolvedValue(skills);
	const queryClient = createTestQueryClient();
	const onInvoke = vi.fn();
	render(
		<QueryClientProvider client={queryClient}>
			<TooltipProvider>
				<ProjectSkillActions
					organizationId={MockChatProject.organization_id}
					projectId={MockChatProject.id}
					onInvoke={onInvoke}
				/>
			</TooltipProvider>
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), queryClient, onInvoke };
};

describe("ProjectSkillActions", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("offers the first three actions by name and the rest under More", async () => {
		// "audit" and "check" sort first, so either would take an inline
		// slot if non-action or disabled skills were offered.
		const { user, onInvoke } = renderActions([
			mockSkill("triage"),
			mockSkill("audit", { action: false }),
			mockSkill("review"),
			mockSkill("check", { enabled: false }),
			mockSkill("release"),
			mockSkill("lint"),
			mockSkill("deploy"),
		]);

		await user.click(await screen.findByRole("button", { name: "deploy" }));
		await user.click(screen.getByRole("button", { name: "lint" }));
		await user.click(screen.getByRole("button", { name: "release" }));
		for (const name of ["review", "triage"]) {
			await user.click(screen.getByRole("button", { name: "More" }));
			await user.click(
				await screen.findByRole("menuitem", { name: new RegExp(`^${name}`) }),
			);
			await waitFor(() => expect(onInvoke).toHaveBeenLastCalledWith(name));
		}

		expect(onInvoke.mock.calls).toEqual([
			["deploy"],
			["lint"],
			["release"],
			["review"],
			["triage"],
		]);
	});

	it("offers nothing when no enabled skill is an action", async () => {
		const { queryClient } = renderActions([
			mockSkill("audit", { action: false }),
			mockSkill("check", { enabled: false }),
		]);

		await waitFor(() =>
			expect(
				queryClient.getQueryData(chatProjectSkillsKey(MockChatProject.id)),
			).toHaveLength(2),
		);
		expect(screen.queryAllByRole("button")).toEqual([]);
	});
});
