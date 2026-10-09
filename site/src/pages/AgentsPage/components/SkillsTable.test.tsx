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
import type { SkillsCopy } from "./SkillsTableView";

const copy: SkillsCopy = {
	noun: "Organization skill",
	title: "Skills",
	description: "Organization skills.",
	emptyDescription: "Add a skill.",
	editorDescription: "Organization skill guidance.",
	archiveName: "organization-skills.zip",
};

const mockReviewSkill = { ...MockSkill, name: "review-sql", enabled: true };

const renderTable = (canEdit: boolean) => {
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([
		mockReviewSkill,
	]);
	const updateSkill = vi
		.spyOn(API.experimental, "updateOrganizationSkill")
		.mockResolvedValue({ ...mockReviewSkill, enabled: false, content: "" });
	renderComponent(
		<QueryClientProvider client={createTestQueryClient()}>
			<SkillsTable
				owner={{ type: "organization", organizationId: MockOrganization.id }}
				copy={copy}
				canEdit={canEdit}
			/>
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), updateSkill };
};

describe("SkillsTable enabled toggle", () => {
	it("sends only the enabled flag", async () => {
		const { user, updateSkill } = renderTable(true);

		await user.click(
			await screen.findByRole("switch", { name: "Enable review-sql" }),
		);

		await waitFor(() => expect(updateSkill).toHaveBeenCalledTimes(1));
		expect(updateSkill.mock.calls[0]).toStrictEqual([
			MockOrganization.id,
			"review-sql",
			{ enabled: false },
		]);
	});

	it("does not send an update when the user cannot edit", async () => {
		const { user, updateSkill } = renderTable(false);

		await user.click(
			await screen.findByRole("switch", {
				name: "Organization skill review-sql",
			}),
		);

		expect(updateSkill).not.toHaveBeenCalled();
	});
});

describe("SkillsTable row menu dialogs", () => {
	it("returns focus to the row menu button when a dialog closes", async () => {
		const { user } = renderTable(true);

		const menuButton = await screen.findByRole("button", { name: "Open menu" });
		await user.click(menuButton);
		await user.click(await screen.findByRole("menuitem", { name: /Delete/ }));
		await screen.findByRole("dialog");
		await user.keyboard("{Escape}");

		await waitFor(() => expect(menuButton).toHaveFocus());
	});

	it("moves focus to the Add skill button after a confirmed delete", async () => {
		const { user } = renderTable(true);
		vi.spyOn(API.experimental, "deleteOrganizationSkill").mockResolvedValue();

		await user.click(await screen.findByRole("button", { name: "Open menu" }));
		await user.click(await screen.findByRole("menuitem", { name: /Delete/ }));
		await user.click(
			await screen.findByRole("button", { name: "Delete skill" }),
		);

		await waitFor(() =>
			expect(document.activeElement).toHaveAccessibleName("Add skill"),
		);
	});

	it("closes the View dialog with its Close button", async () => {
		const { user } = renderTable(false);
		vi.spyOn(API.experimental, "getOrganizationSkillByName").mockResolvedValue({
			...mockReviewSkill,
			content: "---\nname: review-sql\n---\nBody.",
		});

		const menuButton = await screen.findByRole("button", { name: "Open menu" });
		await user.click(menuButton);
		await user.click(await screen.findByRole("menuitem", { name: "View" }));
		await user.click(await screen.findByRole("button", { name: "Close" }));

		await waitFor(() => expect(menuButton).toHaveFocus());
	});
});
