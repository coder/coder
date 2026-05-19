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
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsPersonalSkillsPageView",
	component: AgentSettingsPersonalSkillsPageView,
	args: baseArgs,
} satisfies Meta<typeof AgentSettingsPersonalSkillsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsPersonalSkillsPageView>;

export const Populated: Story = {};

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
				"---\nname: debug-http\ndescription: Debug HTTP handlers.\n---\nInspect request flow and response codes.\n",
			);
		});
	},
};
