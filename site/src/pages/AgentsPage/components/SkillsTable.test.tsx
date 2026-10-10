import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockOrganization } from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import { SkillsTable } from "./SkillsTable";
import {
	fullSkillAccess,
	type SkillAccess,
	type SkillsCopy,
} from "./SkillsTableView";

const copy: SkillsCopy = {
	noun: "Organization skill",
	title: "Skills",
	description: "Organization skills.",
	emptyDescription: "Add a skill.",
	editorDescription: "Organization skill guidance.",
	archiveName: "organization-skills.zip",
};

const mockReviewSkill = { ...MockSkill, name: "review-sql", enabled: true };

const renderTable = (access: SkillAccess) => {
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([
		mockReviewSkill,
	]);
	renderComponent(
		<QueryClientProvider client={createTestQueryClient()}>
			<SkillsTable
				owner={{ type: "organization", organizationId: MockOrganization.id }}
				copy={copy}
				access={access}
			/>
		</QueryClientProvider>,
	);
	return { user: userEvent.setup() };
};

describe("SkillsTable row menu dialogs", () => {
	it("returns focus to the row menu button when a dialog closes", async () => {
		const { user } = renderTable(fullSkillAccess);

		const menuButton = await screen.findByRole("button", { name: "Open menu" });
		await user.click(menuButton);
		await user.click(await screen.findByRole("menuitem", { name: /Delete/ }));
		await screen.findByRole("dialog");
		await user.keyboard("{Escape}");

		await waitFor(() => expect(menuButton).toHaveFocus());
	});
});
