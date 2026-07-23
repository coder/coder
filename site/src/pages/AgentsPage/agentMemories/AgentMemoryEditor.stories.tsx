import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fireEvent, fn, userEvent, within } from "storybook/test";
import type { AgentMemory } from "#/api/typesGenerated";
import { AgentMemoryEditor } from "./AgentMemoryEditor";

const memory: AgentMemory = {
	id: "4ae66d9b-2ec6-4805-a961-daa1059782f3",
	path: "/memory.md",
	content: "# Memory\n\nPrefers concise explanations.",
	created_at: "2026-07-20T10:00:00.000Z",
	updated_at: "2026-07-23T14:30:00.000Z",
};

const callbacks = () => ({
	onDirtyChange: fn(),
	onSave: fn(),
	onReloadLatest: fn(async () => memory),
	onDelete: fn(),
	onBack: fn(),
});

const meta = {
	title: "pages/AgentsPage/AgentMemoryEditor",
	component: AgentMemoryEditor,
	args: {
		memory,
		isSaving: false,
		isConflict: false,
		...callbacks(),
	},
} satisfies Meta<typeof AgentMemoryEditor>;

export default meta;
type Story = StoryObj<typeof AgentMemoryEditor>;

export const Populated: Story = {};

export const EditResetAndSave: Story = {
	args: callbacks(),
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const editor = canvas.getByLabelText("Markdown");
		const save = canvas.getByRole("button", { name: "Save" });
		expect(save).toBeDisabled();

		await userEvent.clear(editor);
		await userEvent.type(editor, "Updated memory");
		expect(save).toBeEnabled();
		expect(args.onDirtyChange).toHaveBeenLastCalledWith(true);

		await userEvent.click(canvas.getByRole("button", { name: "Reset" }));
		expect(editor).toHaveValue(memory.content);
		expect(save).toBeDisabled();

		await userEvent.clear(editor);
		await userEvent.type(editor, "Saved memory");
		await userEvent.click(save);
		expect(args.onSave).toHaveBeenCalledWith("Saved memory");
	},
};

export const StaleConflictReloadsLatest: Story = {
	args: {
		...callbacks(),
		isConflict: true,
		onReloadLatest: fn(async () => ({
			...memory,
			content: "Latest server content",
			updated_at: "2026-07-23T15:00:00.000Z",
		})),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Memory changed elsewhere")).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Reload latest" }),
		);
		expect(args.onReloadLatest).toHaveBeenCalledOnce();
		expect(canvas.getByLabelText("Markdown")).toHaveValue(
			"Latest server content",
		);
	},
};

export const OversizedDraftCannotSave: Story = {
	args: callbacks(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const editor = canvas.getByLabelText("Markdown");
		fireEvent.change(editor, { target: { value: "x".repeat(65_537) } });
		expect(canvas.getByRole("button", { name: "Save" })).toBeDisabled();
		expect(canvas.getByText("65,537 / 65,536 bytes")).toBeVisible();
	},
};

export const ActionsAndMobileBack: Story = {
	args: callbacks(),
	parameters: { viewport: { defaultViewport: "mobile1" } },
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Delete" }));
		expect(args.onDelete).toHaveBeenCalledOnce();
		await userEvent.click(
			canvas.getByRole("button", { name: "Back to memories" }),
		);
		expect(args.onBack).toHaveBeenCalledOnce();
	},
};

export const SaveError: Story = {
	args: {
		...callbacks(),
		saveError: "Could not save the memory.",
	},
};
