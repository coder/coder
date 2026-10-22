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
const mockReviewSkillContent =
	'---\nname: review-sql\ndescription: "Review SQL."\n---\nCheck the queries.\n';

const renderTable = () => {
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([
		mockReviewSkill,
	]);
	const updateSkill = vi
		.spyOn(API.experimental, "updateOrganizationSkill")
		.mockResolvedValue({ ...mockReviewSkill, enabled: false, content: "" });
	const createSkill = vi
		.spyOn(API.experimental, "createOrganizationSkill")
		.mockResolvedValue({ ...mockReviewSkill, content: "" });
	const getSkill = vi
		.spyOn(API.experimental, "getOrganizationSkillByName")
		.mockResolvedValue({
			...mockReviewSkill,
			description: "Review SQL.",
			content: mockReviewSkillContent,
		});
	renderComponent(
		<QueryClientProvider client={createTestQueryClient()}>
			<SkillsTable
				scope={organizationSkillsScope(MockOrganization.id)}
				copy={copy}
				access={fullSkillAccess}
			/>
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), updateSkill, createSkill, getSkill };
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

describe("SkillsTable automatic use", () => {
	it("creates an imported manual-only skill as manual-only", async () => {
		const { user, createSkill } = renderTable();

		await user.click(await screen.findByRole("button", { name: "Add skill" }));
		await user.upload(
			screen.getByLabelText("Upload SKILL.md"),
			new File(
				[
					"---\nname: deploy\ndisable-model-invocation: true\n---\nDeploy it.\n",
				],
				"SKILL.md",
				{ type: "text/markdown" },
			),
		);
		await screen.findByText("Imported SKILL.md");
		await user.click(screen.getByRole("button", { name: "Create skill" }));

		await waitFor(() => expect(createSkill).toHaveBeenCalledTimes(1));
		expect(createSkill.mock.calls[0]).toStrictEqual([
			MockOrganization.id,
			{
				content: "---\nname: deploy\n---\nDeploy it.\n",
				model_invocable: false,
			},
		]);
	});

	it("saves a change to only the automatic-use switch", async () => {
		const { user, updateSkill } = renderTable();

		await user.click(await screen.findByRole("button", { name: "Open menu" }));
		await user.click(await screen.findByRole("menuitem", { name: "Edit" }));
		await user.click(
			await screen.findByRole("switch", {
				name: "Agents can use it automatically",
			}),
		);
		await user.click(screen.getByRole("button", { name: "Save skill" }));

		await waitFor(() => expect(updateSkill).toHaveBeenCalledTimes(1));
		expect(updateSkill.mock.calls[0]).toStrictEqual([
			MockOrganization.id,
			"review-sql",
			{ content: mockReviewSkillContent, model_invocable: false },
		]);
	});

	it("marks a downloaded manual-only skill so re-importing keeps it manual", async () => {
		const { user, getSkill } = renderTable();
		getSkill.mockResolvedValue({
			...mockReviewSkill,
			model_invocable: false,
			content: mockReviewSkillContent,
		});

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
