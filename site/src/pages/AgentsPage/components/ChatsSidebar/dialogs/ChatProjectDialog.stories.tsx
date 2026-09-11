import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { MockChatProject } from "#/testHelpers/entities";
import { ChatProjectDialog } from "./ChatProjectDialog";

const meta = {
	title: "pages/AgentsPage/ChatProjectDialog",
	component: ChatProjectDialog,
	args: {
		organizationId: MockChatProject.organization_id,
		open: true,
		onOpenChange: fn(),
		onSubmit: fn(async () => undefined),
	},
} satisfies Meta<typeof ChatProjectDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Create: Story = {};

export const Edit: Story = {
	args: { project: MockChatProject },
};

export const Submitting: Story = {
	args: {
		onSubmit: fn(() => new Promise<undefined>(() => {})),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText("Name"), "Launch");
		await userEvent.click(canvas.getByRole("button", { name: "Save" }));
	},
};

export const DuplicateNameError: Story = {
	args: {
		onSubmit: fn(async () => {
			throw new globalThis.Error("A project with this name already exists.");
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText("Name"), "Launch");
		await userEvent.click(canvas.getByRole("button", { name: "Save" }));
	},
};
