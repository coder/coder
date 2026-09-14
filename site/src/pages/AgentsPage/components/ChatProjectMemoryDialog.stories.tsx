import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { MockChatProjectMemory } from "#/testHelpers/entities";
import { ChatProjectMemoryDialog } from "./ChatProjectMemoryDialog";

const meta = {
	title: "pages/AgentsPage/ChatProjectMemoryDialog",
	component: ChatProjectMemoryDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		onSubmit: fn(async () => undefined),
	},
} satisfies Meta<typeof ChatProjectMemoryDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Create: Story = {};

export const Edit: Story = {
	args: { memory: MockChatProjectMemory },
};

export const DuplicateNameError: Story = {
	args: {
		onSubmit: fn(async () => {
			throw new globalThis.Error("A memory with this name already exists.");
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText("Name"), "memory");
		await userEvent.type(
			canvas.getByLabelText("Body"),
			"Durable project fact.",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Save" }));
	},
};
