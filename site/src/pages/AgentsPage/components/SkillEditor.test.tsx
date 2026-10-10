import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SKILL_MAX_SIZE_BYTES } from "../utils/skills";
import { SkillEditor } from "./SkillEditor";

const markdown =
	"---\nname: imported-skill\ndescription: Imported guidance.\n---\n\nUse imported instructions.";

const renderEditor = (mode: "create" | "edit" = "create") => {
	const onSubmit = vi.fn();
	render(
		<SkillEditor
			open
			mode={mode}
			noun="Personal skill"
			editorDescription="Personal skill guidance."
			initialValues={
				mode === "create"
					? { name: "", description: "", body: "" }
					: {
							name: "existing-skill",
							description: "Existing guidance.",
							body: "Existing instructions.",
						}
			}
			existingNames={[]}
			isSubmitting={false}
			onOpenChange={vi.fn()}
			onSubmit={onSubmit}
		/>,
	);
	return { user: userEvent.setup(), onSubmit };
};

describe("SkillEditor file import", () => {
	it.each(["create", "edit"] as const)(
		"imports a file before submitting in %s mode",
		async (mode) => {
			const { user, onSubmit } = renderEditor(mode);
			await user.upload(
				screen.getByLabelText("Upload SKILL.md"),
				new File([markdown], "SKILL.md", { type: "text/markdown" }),
			);
			await screen.findByText("Imported SKILL.md");
			expect(onSubmit).not.toHaveBeenCalled();

			await user.click(
				screen.getByRole("button", {
					name: mode === "create" ? "Create skill" : "Save skill",
				}),
			);
			const name = mode === "create" ? "imported-skill" : "existing-skill";
			await waitFor(() =>
				expect(onSubmit).toHaveBeenCalledWith(
					{
						name,
						description: "Imported guidance.",
						body: "Use imported instructions.",
					},
					`---\nname: ${name}\ndescription: "Imported guidance."\n---\nUse imported instructions.\n`,
				),
			);
		},
	);

	it.each([
		{
			content: "---\ndescription: Missing name\n---\nBody",
			error: "Could not parse SKILL.md",
		},
		{ content: " \n", error: "File is empty" },
		{
			content: "x".repeat(SKILL_MAX_SIZE_BYTES + 1),
			error: "File is too large",
		},
	])("preserves edits when $error", async ({ content, error }) => {
		const { user, onSubmit } = renderEditor("edit");
		await user.clear(screen.getByRole("textbox", { name: "Body" }));
		await user.type(
			screen.getByRole("textbox", { name: "Body" }),
			"Unsaved instructions.",
		);
		await user.upload(
			screen.getByLabelText("Upload SKILL.md"),
			new File([content], "SKILL.md", { type: "text/markdown" }),
		);
		await screen.findByText(error);
		await user.click(screen.getByRole("button", { name: "Save skill" }));
		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith(
				{
					name: "existing-skill",
					description: "Existing guidance.",
					body: "Unsaved instructions.",
				},
				'---\nname: existing-skill\ndescription: "Existing guidance."\n---\nUnsaved instructions.\n',
			),
		);
	});

	it("allows selecting the same file again after a read failure", async () => {
		const { user, onSubmit } = renderEditor();
		const file = new File([markdown], "SKILL.md", { type: "text/markdown" });
		file.text = vi
			.fn<() => Promise<string>>()
			.mockRejectedValueOnce(new Error("Read failed"))
			.mockResolvedValue(markdown);

		await user.upload(screen.getByLabelText("Upload SKILL.md"), file);
		await screen.findByText("Could not read file");
		await user.upload(screen.getByLabelText("Upload SKILL.md"), file);
		await screen.findByText("Imported SKILL.md");
		await user.click(screen.getByRole("button", { name: "Create skill" }));
		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith(
				{
					name: "imported-skill",
					description: "Imported guidance.",
					body: "Use imported instructions.",
				},
				'---\nname: imported-skill\ndescription: "Imported guidance."\n---\nUse imported instructions.\n',
			),
		);
	});
});
