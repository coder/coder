import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { mockApiError } from "#/testHelpers/entities";
import { InlineEditField } from "./InlineEditField";

const meta: Meta<typeof InlineEditField> = {
	title: "pages/AgentsPage/ProjectPage/InlineEditField",
	component: InlineEditField,
	decorators: [
		(Story) => (
			<div className="max-w-md">
				<Story />
			</div>
		),
	],
	args: {
		label: "Name",
		value: "Launch",
		onSave: async () => undefined,
	},
};

export default meta;
type Story = StoryObj<typeof InlineEditField>;

export const Default: Story = {};

export const Empty: Story = { args: { value: "" } };

export const ReadOnly: Story = { args: { onSave: undefined } };

export const Multiline: Story = {
	args: {
		label: "Description",
		multiline: true,
		value: "Chats for the launch work.\nKeep replies short.",
	},
};

export const Editing: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Edit Name" }),
		);
	},
};

export const InvalidDraft: Story = {
	args: {
		validate: (value) => (value === "" ? "Name is required." : undefined),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Edit Name" }));
		await userEvent.clear(canvas.getByRole("textbox", { name: "Name" }));
	},
};

export const Saving: Story = {
	args: { onSave: () => new Promise(() => undefined) },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Edit Name" }));
		await userEvent.type(canvas.getByRole("textbox", { name: "Name" }), "!");
		await userEvent.click(canvas.getByRole("button", { name: "Save Name" }));
	},
};

export const SaveError: Story = {
	args: {
		onSave: async () => {
			throw mockApiError({ message: "The server rejected the name." });
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Edit Name" }));
		await userEvent.type(canvas.getByRole("textbox", { name: "Name" }), "!");
		await userEvent.click(canvas.getByRole("button", { name: "Save Name" }));
	},
};

export const MultilineEditing: Story = {
	args: {
		label: "Description",
		multiline: true,
		value: "Chats for the launch work.\nKeep replies short.",
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Edit Description" }),
		);
	},
};
