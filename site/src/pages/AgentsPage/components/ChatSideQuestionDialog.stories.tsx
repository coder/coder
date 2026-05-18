import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { ChatSideQuestionDialog } from "./ChatSideQuestionDialog";

const meta: Meta<typeof ChatSideQuestionDialog> = {
	title: "pages/AgentsPage/ChatSideQuestionDialog",
	component: ChatSideQuestionDialog,
	args: {
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ChatSideQuestionDialog>;

export const StreamingInitial: Story = {
	args: {
		state: { status: "streaming", question: "What changed?", answer: "" },
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement.ownerDocument.body);
		expect(canvas.getByRole("dialog")).toBeInTheDocument();
		expect(canvas.getByText("Answering side question...")).toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button", { name: "Cancel" }));
		expect(args.onClose).toHaveBeenCalled();
	},
};

export const StreamingPartial: Story = {
	args: {
		state: {
			status: "streaming",
			question: "What changed?",
			answer: "The assistant is explaining",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement.ownerDocument.body);
		expect(canvas.getByText("The assistant is explaining")).toBeInTheDocument();
	},
};

export const Success: Story = {
	args: {
		state: {
			status: "success",
			question: "What changed?",
			answer:
				"The chat is discussing side questions. This answer is local to the overlay and is not added to the transcript.",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement.ownerDocument.body);
		expect(canvas.getByRole("dialog")).toBeInTheDocument();
		expect(canvas.getByText("What changed?")).toBeInTheDocument();
		expect(
			canvas.getByText(/not added to the transcript/i),
		).toBeInTheDocument();
	},
};

export const Failed: Story = {
	args: {
		state: {
			status: "error",
			question: "What changed?",
			message: "Failed to answer side question.",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement.ownerDocument.body);
		expect(canvas.getByRole("alert")).toHaveTextContent(
			"Failed to answer side question.",
		);
	},
};
