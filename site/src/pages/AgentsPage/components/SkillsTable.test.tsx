import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { toast } from "sonner";
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
const mockDocsSkill = {
	...MockSkill,
	id: "skill-docs-style",
	name: "docs-style",
	enabled: true,
};

const renderTable = (canEdit: boolean, skills = [mockReviewSkill]) => {
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue(skills);
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

	it("ignores a repeat click while that skill's update is pending", async () => {
		const { user, updateSkill } = renderTable(true, [
			mockReviewSkill,
			mockDocsSkill,
		]);
		updateSkill.mockReturnValue(new Promise(() => {}));

		await user.click(
			await screen.findByRole("switch", { name: "Enable review-sql" }),
		);
		await user.click(screen.getByRole("switch", { name: "Enable docs-style" }));
		await user.click(screen.getByRole("switch", { name: "Enable review-sql" }));

		expect(updateSkill.mock.calls.map(([, name]) => name)).toStrictEqual([
			"review-sql",
			"docs-style",
		]);
	});

	it("toggles the same skill again after its update settles", async () => {
		const { user, updateSkill } = renderTable(true);
		const toggle = await screen.findByRole("switch", {
			name: "Enable review-sql",
		});

		await user.click(toggle);
		await waitFor(() => expect(toggle).not.toBeChecked());
		await user.click(toggle);

		await waitFor(() => expect(updateSkill).toHaveBeenCalledTimes(2));
	});

	it("refetches the list when the skill was deleted elsewhere", async () => {
		const { user, updateSkill } = renderTable(true);
		updateSkill.mockRejectedValue({
			isAxiosError: true,
			response: { status: 404, data: { message: "Resource not found." } },
		});
		const infoToast = vi.spyOn(toast, "info");

		await user.click(
			await screen.findByRole("switch", { name: "Enable review-sql" }),
		);

		await waitFor(() =>
			expect(API.experimental.getOrganizationSkills).toHaveBeenCalledTimes(2),
		);
		expect(infoToast).toHaveBeenCalledWith(
			"That organization skill was deleted before your change was saved.",
		);
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

describe("SkillsTable create dialog", () => {
	it("returns focus to the empty-state Add skill button on Escape", async () => {
		const { user } = renderTable(true, []);

		const emptyStateButton = await within(screen.getByRole("table")).findByRole(
			"button",
			{ name: "Add skill" },
		);
		await user.click(emptyStateButton);
		await screen.findByRole("dialog");
		await user.keyboard("{Escape}");

		await waitFor(() => expect(emptyStateButton).toHaveFocus());
	});

	it("moves focus to the header Add skill button after creating the first skill", async () => {
		const { user } = renderTable(true, []);
		vi.spyOn(API.experimental, "createOrganizationSkill").mockResolvedValue({
			...mockReviewSkill,
			content: "---\nname: review-sql\n---\nBody.",
		});

		await user.click(
			await within(screen.getByRole("table")).findByRole("button", {
				name: "Add skill",
			}),
		);
		await user.type(await screen.findByLabelText("Name"), "review-sql");
		await user.type(screen.getByLabelText("Body"), "Body.");
		await user.click(screen.getByRole("button", { name: "Create skill" }));

		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Add skill" })).toHaveFocus(),
		);
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
