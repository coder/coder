import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { UserSkillMetadata } from "#/api/typesGenerated";
import {
	AgentSettingsPersonalSkillsPageView,
	type AgentSettingsPersonalSkillsPageViewProps,
} from "./AgentSettingsPersonalSkillsPageView";

const buildSkill = (
	overrides: Partial<UserSkillMetadata> & Pick<UserSkillMetadata, "name">,
): UserSkillMetadata => ({
	id: overrides.id ?? `skill-${overrides.name}`,
	name: overrides.name,
	description: overrides.description ?? "Reusable guidance for agents.",
	created_at: overrides.created_at ?? "2026-05-01T12:00:00.000Z",
	updated_at: overrides.updated_at ?? "2026-05-03T15:30:00.000Z",
});

const skills = [
	buildSkill({
		name: "review-sql",
		description: "Review SQL changes for query and index risks.",
	}),
	buildSkill({
		name: "write-release-notes",
		description: "Draft concise release notes from a change list.",
		updated_at: "2026-05-04T09:15:00.000Z",
	}),
];

const firstSkill = skills[0] ?? buildSkill({ name: "review-sql" });

const baseArgs: AgentSettingsPersonalSkillsPageViewProps = {
	skills,
	error: undefined,
	isLoading: false,
	isRetrying: false,
	onRetry: fn(),
	onCreate: fn(),
	onEdit: fn(),
	onDelete: fn(),
	onDownload: fn(),
	onExportAll: fn(),
};

const meta = {
	title:
		"pages/AgentsPage/AgentSettingsPage/AgentSettingsPersonalSkillsPage/AgentSettingsPersonalSkillsPageView",
	component: AgentSettingsPersonalSkillsPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsPersonalSkillsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsPersonalSkillsPageView>;

export const Populated: Story = {};

export const DownloadingSkill: Story = {
	args: {
		downloadingSkillName: "review-sql",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const row = canvas.getByRole("row", { name: /review-sql/ });
		await userEvent.click(
			within(row).getByRole("button", { name: "Open menu" }),
		);
		const menu = await body.findByRole("menu");
		await expect(
			within(menu).getByRole("menuitem", { name: "Download" }),
		).toHaveAttribute("aria-disabled", "true");
		await expect(
			within(menu).getByRole("menuitem", { name: "Edit" }),
		).not.toHaveAttribute("aria-disabled");
	},
};

export const ExportingAll: Story = {
	args: {
		isExportingAll: true,
	},
};

export const RowMenuActions: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const row = canvas.getByRole("row", { name: /review-sql/ });
		const trigger = within(row).getByRole("button", { name: "Open menu" });

		await userEvent.click(trigger);
		await userEvent.click(
			within(await body.findByRole("menu")).getByRole("menuitem", {
				name: "Download",
			}),
		);
		await waitFor(() => {
			expect(args.onDownload).toHaveBeenCalledWith(
				expect.objectContaining({ name: "review-sql" }),
			);
		});

		await userEvent.click(trigger);
		await userEvent.click(
			within(await body.findByRole("menu")).getByRole("menuitem", {
				name: "Edit",
			}),
		);
		await waitFor(() => {
			expect(args.onEdit).toHaveBeenCalledWith("review-sql");
		});

		await userEvent.click(trigger);
		await userEvent.click(
			within(await body.findByRole("menu")).getByRole("menuitem", {
				name: /Delete/,
			}),
		);
		await waitFor(() => {
			expect(args.onDelete).toHaveBeenCalledWith(
				expect.objectContaining({ name: "review-sql" }),
			);
		});
	},
};

export const ExportsAllSkills: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Export all" }));

		await waitFor(() => {
			expect(args.onExportAll).toHaveBeenCalled();
		});
	},
};

export const Loading: Story = {
	args: {
		skills: [],
		isLoading: true,
	},
};

export const Empty: Story = {
	args: {
		skills: [],
	},
};

export const ListError: Story = {
	args: {
		skills: [],
		error: new Error("Failed to load personal skills."),
	},
};

export const RefetchErrorKeepsRows: Story = {
	args: {
		error: new Error("Failed to load personal skills."),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("row", { name: /review-sql/ })).toBeVisible();
		await expect(
			canvas.getByText("Failed to load personal skills."),
		).toBeVisible();
	},
};

export const CreateDialogOpen: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: skills.map((skill) => skill.name),
			isSubmitting: false,
			onSubmit: fn(),
			onClose: fn(),
		},
	},
};

export const EditDialogOpen: Story = {
	args: {
		editorState: {
			mode: "edit",
			initialValues: {
				name: "review-sql",
				description: "Review SQL changes for query and index risks.",
				body: "Check query plans, missing indexes, and transaction boundaries.",
			},
			existingNames: skills.map((skill) => skill.name),
			isLoading: false,
			isRetrying: false,
			isSubmitting: false,
			onRetry: fn(),
			onSubmit: fn(),
			onClose: fn(),
		},
	},
};

export const EditDialogLoading: Story = {
	args: {
		editorState: {
			mode: "edit",
			existingNames: skills.map((skill) => skill.name),
			isLoading: true,
			isRetrying: false,
			isSubmitting: false,
			onRetry: fn(),
			onSubmit: fn(),
			onClose: fn(),
		},
	},
};

export const EditDialogLoadError: Story = {
	args: {
		editorState: {
			mode: "edit",
			existingNames: skills.map((skill) => skill.name),
			loadError: new Error("Failed to load personal skill."),
			isLoading: false,
			isRetrying: true,
			isSubmitting: false,
			onRetry: fn(),
			onSubmit: fn(),
			onClose: fn(),
		},
	},
};

export const ImportSkillMarkdownPopulatesCreateFields: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: skills.map((skill) => skill.name),
			isSubmitting: false,
			onSubmit: fn(),
			onClose: fn(),
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const dialogCanvas = within(dialog);
		const importInput = dialogCanvas.getByLabelText("Import from SKILL.md");

		await userEvent.click(importInput);
		await userEvent.paste(
			"---\nname: imported-skill\ndescription: Imported guidance.\n---\n\nUse imported instructions.",
		);

		await waitFor(() => {
			expect(dialogCanvas.getByLabelText("Name")).toHaveValue("imported-skill");
			expect(dialogCanvas.getByLabelText("Description")).toHaveValue(
				"Imported guidance.",
			);
			expect(dialogCanvas.getByLabelText("Body")).toHaveValue(
				"Use imported instructions.",
			);
			expect(dialogCanvas.getByText("Imported SKILL.md")).toBeVisible();
		});
	},
};

export const ImportSkillMarkdownShowsParseError: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: skills.map((skill) => skill.name),
			isSubmitting: false,
			onSubmit: fn(),
			onClose: fn(),
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const dialogCanvas = within(dialog);
		const importInput = dialogCanvas.getByLabelText("Import from SKILL.md");

		await userEvent.click(importInput);
		await userEvent.paste("---\ndescription: Missing name\n---\nBody");

		await waitFor(() => {
			expect(dialogCanvas.getByText("Could not parse SKILL.md")).toBeVisible();
			expect(dialogCanvas.getByText("Skill name is required.")).toBeVisible();
			expect(dialogCanvas.getByLabelText("Name")).toHaveValue("");
			expect(dialogCanvas.getByLabelText("Description")).toHaveValue("");
			expect(dialogCanvas.getByLabelText("Body")).toHaveValue("");
		});
	},
};

export const ImportSkillMarkdownKeepsEditName: Story = {
	args: {
		editorState: {
			mode: "edit",
			initialValues: {
				name: "review-sql",
				description: "Review SQL changes for query and index risks.",
				body: "Check query plans, missing indexes, and transaction boundaries.",
			},
			existingNames: skills.map((skill) => skill.name),
			isLoading: false,
			isRetrying: false,
			isSubmitting: false,
			onRetry: fn(),
			onSubmit: fn(),
			onClose: fn(),
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const dialogCanvas = within(dialog);
		const importInput = dialogCanvas.getByLabelText("Import from SKILL.md");

		await userEvent.click(importInput);
		await userEvent.paste(
			"---\nname: pasted-name\ndescription: New description.\n---\n\nNew body.",
		);

		await waitFor(() => {
			expect(dialogCanvas.getByLabelText("Name")).toHaveValue("review-sql");
			expect(dialogCanvas.getByLabelText("Description")).toHaveValue(
				"New description.",
			);
			expect(dialogCanvas.getByLabelText("Body")).toHaveValue("New body.");
			expect(
				dialogCanvas.getByText(
					"Updated description and body fields. Kept the existing name.",
				),
			).toBeVisible();
		});
	},
};

export const DeleteConfirmationOpen: Story = {
	args: {
		deleteState: {
			skill: firstSkill,
			isDeleting: false,
			onConfirm: fn(),
			onClose: fn(),
		},
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const dialogCanvas = within(dialog);

		await userEvent.click(
			dialogCanvas.getByRole("button", { name: "Delete skill" }),
		);

		await waitFor(() => {
			expect(args.deleteState?.onConfirm).toHaveBeenCalled();
		});
	},
};

export const CreateDialogSubmitError: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: skills.map((skill) => skill.name),
			submitError: {
				message: "Failed to create personal skill.",
				detail: "Skill content is invalid.",
			},
			isSubmitting: false,
			onSubmit: fn(),
			onClose: fn(),
		},
	},
};

export const EditDialogSubmitError: Story = {
	args: {
		editorState: {
			mode: "edit",
			initialValues: {
				name: "review-sql",
				description: "Review SQL changes for query and index risks.",
				body: "Check query plans, missing indexes, and transaction boundaries.",
			},
			existingNames: skills.map((skill) => skill.name),
			isLoading: false,
			isRetrying: false,
			submitError: {
				message: "Failed to save personal skill.",
				detail: "That personal skill was not found.",
			},
			isSubmitting: false,
			onRetry: fn(),
			onSubmit: fn(),
			onClose: fn(),
		},
	},
};

export const DeleteConfirmationError: Story = {
	args: {
		deleteState: {
			skill: firstSkill,
			error: {
				message: "Failed to delete personal skill.",
				detail: "That personal skill was not found.",
			},
			isDeleting: false,
			onConfirm: fn(),
			onClose: fn(),
		},
	},
};

export const InvalidNameIsRejected: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: skills.map((skill) => skill.name),
			isSubmitting: false,
			onSubmit: fn(),
			onClose: fn(),
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const dialogCanvas = within(dialog);
		const nameInput = dialogCanvas.getByLabelText("Name");
		const bodyInput = dialogCanvas.getByLabelText("Body");

		await userEvent.type(nameInput, "Bad Name");
		await userEvent.click(bodyInput);

		await waitFor(() => {
			expect(nameInput).toHaveAttribute("aria-invalid", "true");
			expect(
				dialogCanvas.getByRole("button", { name: "Create skill" }),
			).toBeDisabled();
		});
	},
};

export const DuplicateNameIsRejected: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: skills.map((skill) => skill.name),
			isSubmitting: false,
			onSubmit: fn(),
			onClose: fn(),
		},
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const dialogCanvas = within(dialog);
		const nameInput = dialogCanvas.getByLabelText("Name");
		const bodyInput = dialogCanvas.getByLabelText("Body");

		await userEvent.type(nameInput, "review-sql");
		await userEvent.click(bodyInput);

		await waitFor(() => {
			expect(
				dialogCanvas.getByText("A skill with this name already exists."),
			).toBeVisible();
			expect(
				dialogCanvas.getByRole("button", { name: "Create skill" }),
			).toBeDisabled();
		});
	},
};

export const SubmitsCreateDialog: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: skills.map((skill) => skill.name),
			isSubmitting: false,
			onSubmit: fn(),
			onClose: fn(),
		},
	},
	play: async ({ canvasElement, args }) => {
		const body = within(canvasElement.ownerDocument.body);
		const dialog = await body.findByRole("dialog");
		const dialogCanvas = within(dialog);

		await userEvent.type(dialogCanvas.getByLabelText("Name"), "debug-http");
		await userEvent.type(
			dialogCanvas.getByLabelText("Description"),
			"Debug HTTP handlers.",
		);
		await userEvent.type(
			dialogCanvas.getByLabelText("Body"),
			"Inspect request flow and response codes.",
		);
		await userEvent.click(
			dialogCanvas.getByRole("button", { name: "Create skill" }),
		);

		await waitFor(() => {
			expect(args.editorState?.onSubmit).toHaveBeenCalledWith(
				{
					name: "debug-http",
					description: "Debug HTTP handlers.",
					body: "Inspect request flow and response codes.",
				},
				'---\nname: debug-http\ndescription: "Debug HTTP handlers."\n---\nInspect request flow and response codes.\n',
			);
		});
	},
};
