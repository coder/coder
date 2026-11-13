import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { saveAs } from "file-saver";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockOrganization } from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import {
	fullSkillAccess,
	organizationSkillsScope,
	parseSkillMarkdown,
	type SkillsCopy,
} from "../utils/skills";
import { SkillsTable } from "./SkillsTable";

vi.mock("file-saver", () => ({ saveAs: vi.fn() }));

const copy: SkillsCopy = {
	noun: "Organization skill",
	title: "Skills",
	description: "Organization skills.",
	emptyDescription: "Add a skill.",
	editorDescription: "Organization skill guidance.",
	archiveName: "organization-skills.zip",
};

const mockReviewSkill = { ...MockSkill, name: "review-sql", enabled: true };

const renderTable = () => {
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([
		mockReviewSkill,
	]);
	const updateSkill = vi
		.spyOn(API.experimental, "updateOrganizationSkill")
		.mockResolvedValue({ ...mockReviewSkill, enabled: false, content: "" });
	renderComponent(
		<QueryClientProvider client={createTestQueryClient()}>
			<SkillsTable
				scope={organizationSkillsScope(MockOrganization.id)}
				copy={copy}
				access={fullSkillAccess}
			/>
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), updateSkill };
};

describe("SkillsTable enabled toggle", () => {
	it("sends only the enabled flag", async () => {
		const { user, updateSkill } = renderTable();

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
});

describe("SkillsTable row menu dialogs", () => {
	it("returns focus to the row menu button when a dialog closes", async () => {
		const { user } = renderTable();

		const menuButton = await screen.findByRole("button", { name: "Open menu" });
		await user.click(menuButton);
		await user.click(await screen.findByRole("menuitem", { name: /Delete/ }));
		await screen.findByRole("dialog");
		await user.keyboard("{Escape}");

		await waitFor(() => expect(menuButton).toHaveFocus());
	});
});

describe("SkillsTable download", () => {
	it("marks a downloaded manual-only skill as manual-only", async () => {
		vi.spyOn(API.experimental, "getOrganizationSkillByName").mockResolvedValue({
			...mockReviewSkill,
			model_invocable: false,
			content: "---\nname: review-sql\n---\nCheck the queries.\n",
		});
		const { user } = renderTable();

		await user.click(await screen.findByRole("button", { name: "Open menu" }));
		await user.click(await screen.findByRole("menuitem", { name: "Download" }));

		await waitFor(() => expect(saveAs).toHaveBeenCalledTimes(1));
		const [file] = vi.mocked(saveAs).mock.calls[0];
		if (!(file instanceof Blob)) {
			throw new Error("Expected the download to be a Blob.");
		}
		expect(parseSkillMarkdown(await file.text()).modelInvocable).toBe(false);
	});
});
