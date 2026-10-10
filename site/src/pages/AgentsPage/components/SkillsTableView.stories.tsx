import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { MockSkill } from "#/testHelpers/skills";
import {
	fullSkillAccess,
	SkillsTableView,
	type SkillsTableViewProps,
} from "./SkillsTableView";

const MockReviewSQLSkill = {
	...MockSkill,
	id: "skill-review-sql",
	name: "review-sql",
	description: "Review SQL changes for query and index risks.",
	created_at: "2026-05-01T12:00:00.000Z",
	updated_at: "2026-05-03T15:30:00.000Z",
};

const MockReleaseNotesSkill = {
	...MockSkill,
	id: "skill-write-release-notes",
	name: "write-release-notes",
	description: "Draft concise release notes from a change list.",
	enabled: false,
	created_at: "2026-05-01T12:00:00.000Z",
	updated_at: "2026-05-04T09:15:00.000Z",
};

const MockTestAuditSkill = {
	...MockSkill,
	id: "skill-test-audit",
	name: "test-audit",
	description:
		"Invoke whenever writing, changing, reviewing, or sweeping tests in coder/coder. Authoring gate for new tests plus audit workflow for low-value, implementation-coupled, duplicative, or test-only-production-seam-driven tests.",
};

const MockFrontendReviewSkill = {
	...MockSkill,
	id: "skill-frontend-accessibility-and-regression-review",
	name: "frontend-accessibility-and-regression-review",
	description:
		"Review frontend changes for accessibility, loading and error states, reusable components, and regression coverage before opening a pull request.",
};

const MockDebugHTTPSkill = {
	...MockSkill,
	id: "skill-debug-http",
	name: "debug-http",
	description: "",
};

const MockPersonalSkills = [MockReviewSQLSkill, MockReleaseNotesSkill];

const baseArgs: SkillsTableViewProps = {
	owner: { type: "user", user: "me" },
	skills: MockPersonalSkills,
	copy: {
		noun: "Personal skill",
		title: "Personal skills",
		description:
			"Reusable instructions your agents can pick when they need specialized guidance. Personal skills hold a single SKILL.md file. For richer skills with supporting files, add them to your repo under `.agents/skills/` or load them from a workspace.",
		emptyDescription:
			"Create a personal skill to save reusable agent guidance for your workflows.",
		editorDescription:
			"Personal skills are available to your agents and stored as a single SKILL.md file with frontmatter. For richer skills with supporting files, add them to your repo under `.agents/skills/` or load them from a workspace.",
		archiveName: "personal-skills.zip",
	},
	access: fullSkillAccess,
	error: undefined,
	isLoading: false,
	isRetrying: false,
	onRetry: fn(),
	onCreate: fn(),
	onEdit: fn(),
	onDelete: fn(),
	onDownload: fn(),
	onExportAll: fn(),
	isExportingAll: false,
};

const meta = {
	title: "pages/AgentsPage/components/SkillsTableView",
	component: SkillsTableView,
	args: baseArgs,
} satisfies Meta<typeof SkillsTableView>;

export default meta;
type Story = StoryObj<typeof SkillsTableView>;

export const Populated: Story = {};

export const LongDescription: Story = {
	args: {
		skills: [
			MockTestAuditSkill,
			...MockPersonalSkills,
			MockFrontendReviewSkill,
			MockDebugHTTPSkill,
		],
	},
};

export const LongDescriptionNarrow: Story = {
	...LongDescription,
	globals: { viewport: { value: "ipad" } },
};

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

export const LoadingNarrow: Story = {
	...Loading,
	globals: { viewport: { value: "ipad" } },
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
};

export const CreateDialogOpen: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: MockPersonalSkills.map((skill) => skill.name),
			isSubmitting: false,
			onSubmit: fn(),
			onClose: fn(),
		},
	},
};

export const UploadSkillMarkdown: Story = {
	args: CreateDialogOpen.args,
	play: async ({ canvasElement }) => {
		const dialog = within(
			await within(canvasElement.ownerDocument.body).findByRole("dialog"),
		);
		await userEvent.upload(
			dialog.getByLabelText("Upload SKILL.md"),
			new File(
				[
					"---\nname: imported-skill\ndescription: Imported guidance.\n---\n\nUse imported instructions.",
				],
				"SKILL.md",
				{ type: "text/markdown" },
			),
		);
		await dialog.findByText("Imported SKILL.md");
	},
};

export const UploadEmptyFile: Story = {
	args: CreateDialogOpen.args,
	play: async ({ canvasElement }) => {
		const dialog = within(
			await within(canvasElement.ownerDocument.body).findByRole("dialog"),
		);
		await userEvent.upload(
			dialog.getByLabelText("Upload SKILL.md"),
			new File([], "SKILL.md", { type: "text/markdown" }),
		);
		await dialog.findByText("File is empty");
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
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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

		// Wait for the import to populate the create fields so the snapshot
		// captures the imported values.
		await dialogCanvas.findByText("Imported SKILL.md");
	},
};

export const ImportSkillMarkdownShowsParseError: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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

		// Wait for the parse error to render so the snapshot captures it.
		await dialogCanvas.findByText("Could not parse SKILL.md");
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
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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

		// Wait for the import confirmation so the snapshot captures the
		// updated fields with the kept name.
		await dialogCanvas.findByText(
			"Updated description and body fields. Kept the existing name.",
		);
	},
};

export const DeleteConfirmationOpen: Story = {
	args: {
		deleteState: {
			skill: MockReviewSQLSkill,
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
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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
			skill: MockReviewSQLSkill,
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
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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

		// Wait for validation to run so the snapshot captures the rejected
		// state.
		await dialogCanvas.findByText(/kebab-case/i);
	},
};

export const DuplicateNameIsRejected: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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

		// Wait for the duplicate-name validation so the snapshot captures
		// the rejected state.
		await dialogCanvas.findByText("A skill with this name already exists.");
	},
};

export const SubmitsCreateDialog: Story = {
	args: {
		editorState: {
			mode: "create",
			initialValues: { name: "", description: "", body: "" },
			existingNames: MockPersonalSkills.map((skill) => skill.name),
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
