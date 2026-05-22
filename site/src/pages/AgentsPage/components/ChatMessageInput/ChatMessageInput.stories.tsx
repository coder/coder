import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatMessageInput } from "./ChatMessageInput";

const now = "2026-05-08T00:00:00Z";

const mockSkills: TypesGen.UserSkillMetadata[] = [
	{
		id: "skill-reviewer",
		name: "reviewer",
		description: "Review changed files and suggest fixes.",
		created_at: now,
		updated_at: now,
	},
	{
		id: "skill-docs",
		name: "docs",
		description: "Draft docs for user-facing behavior.",
		created_at: now,
		updated_at: now,
	},
	{
		id: "skill-plan",
		name: "plan",
		description: "",
		created_at: now,
		updated_at: now,
	},
];

const meta: Meta<typeof ChatMessageInput> = {
	title: "components/ChatMessageInput/ChatMessageInput",
	component: ChatMessageInput,
	args: {
		"aria-label": "Chat message input",
		placeholder: "Message the agent",
		personalSkillsOverride: mockSkills,
		onChange: fn(),
		onEnter: fn(),
	},
	decorators: [
		(Story) => (
			<div className="w-[520px] space-y-3 rounded-md border border-border border-solid p-4">
				<button type="button" className="text-content-secondary text-sm">
					Outside target
				</button>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ChatMessageInput>;

const findVisibleText = async (text: string) => {
	let visibleElement: HTMLElement | undefined;
	await waitFor(() => {
		const matches = within(document.body).queryAllByText(text);
		visibleElement = matches.find(
			(element) => element.getClientRects().length > 0,
		);
		expect(visibleElement).toBeDefined();
	});
	return visibleElement as HTMLElement;
};

const expectNoVisibleText = async (text: string) => {
	await waitFor(() => {
		const matches = within(document.body).queryAllByText(text);
		expect(
			matches.every((element) => element.getClientRects().length === 0),
		).toBe(true);
	});
};

const editorFromCanvas = (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	return canvas.getByTestId("chat-message-input");
};

const typeInEditor = async (canvasElement: HTMLElement, text: string) => {
	const editor = editorFromCanvas(canvasElement);
	await userEvent.click(editor);
	await userEvent.keyboard(text);
	return editor;
};

export const Closed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByTestId("chat-message-input")).toBeVisible();
		await expectNoVisibleText("/reviewer");
	},
};

export const OpensWithSkills: Story = {
	play: async ({ canvasElement }) => {
		await typeInEditor(canvasElement, "/");
		expect(await findVisibleText("/reviewer")).toBeDefined();
		expect(
			await findVisibleText("Review changed files and suggest fixes."),
		).toBeDefined();
	},
};

export const EmptySkills: Story = {
	args: {
		personalSkillsOverride: [],
		onEnter: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const editor = await typeInEditor(canvasElement, "/");
		expect(await findVisibleText("No personal skills found.")).toBeDefined();
		await userEvent.keyboard("{Enter}");
		expect(args.onEnter).not.toHaveBeenCalled();
		expect(editor.textContent).toBe("/");
	},
};

export const FiltersByQuery: Story = {
	play: async ({ canvasElement }) => {
		await typeInEditor(canvasElement, "/rev");
		expect(await findVisibleText("/reviewer")).toBeDefined();
		await expectNoVisibleText("/docs");
	},
};

export const EnterSelectsSkill: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/rev");
		await findVisibleText("/reviewer");
		await userEvent.keyboard("{Enter}");
		await waitFor(() => {
			expect(editor.textContent).toBe("/reviewer");
		});
		await expectNoVisibleText("Review changed files and suggest fixes.");
	},
};

export const ArrowKeysSelectHighlightedSkill: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/");
		await findVisibleText("/docs");
		await userEvent.keyboard("{ArrowDown}{Enter}");
		await waitFor(() => {
			expect(editor.textContent).toBe("/plan");
		});
	},
};

export const TabSelectsSkill: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/rev");
		await findVisibleText("/reviewer");
		await userEvent.keyboard("{Tab}");
		await waitFor(() => {
			expect(editor.textContent).toBe("/reviewer");
		});
		await expectNoVisibleText("Review changed files and suggest fixes.");
	},
};

export const ClickSelectsSkill: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/rev");
		await userEvent.click(await findVisibleText("/reviewer"));
		await waitFor(() => {
			expect(editor.textContent).toBe("/reviewer");
		});
		await expectNoVisibleText("Review changed files and suggest fixes.");
	},
};

export const EmptyDescriptionInsertsNameOnly: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/pla");
		await findVisibleText("/plan");
		await userEvent.keyboard("{Enter}");
		await waitFor(() => {
			expect(editor.textContent).toBe("/plan");
		});
	},
};

export const SlashInsideUrlDoesNotOpen: Story = {
	play: async ({ canvasElement }) => {
		await typeInEditor(canvasElement, "https://");
		await expectNoVisibleText("/reviewer");
	},
};

export const EscapeClosesWithoutReplacing: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/");
		await findVisibleText("/reviewer");
		await userEvent.keyboard("{Escape}");
		await expectNoVisibleText("/reviewer");
		await waitFor(() => {
			expect(editor).toHaveFocus();
		});
		expect(editor.textContent).toBe("/");
		await userEvent.keyboard("r");
		await expectNoVisibleText("/reviewer");
		expect(editor.textContent).toBe("/r");
	},
};

export const OutsideClickClosesWithoutReplacing: Story = {
	play: async ({ canvasElement }) => {
		const editor = await typeInEditor(canvasElement, "/");
		await findVisibleText("/reviewer");
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Outside target" }),
		);
		await expectNoVisibleText("/reviewer");
		expect(editor.textContent).toBe("/");
	},
};
