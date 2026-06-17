import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { ToolCall } from "./ToolCall";

const meta: Meta = {
	title: "pages/AgentsPage/ChatElements/tools/ToolCall",
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl py-6 font-sans text-xs">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = {
	render: () => (
		<ToolCall.Root status="running" hasContent={false}>
			<ToolCall.Header iconName="read_file" label="Reading README.md" />
		</ToolCall.Root>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Reading README.md")).toBeVisible();
		expect(canvas.queryByRole("button")).not.toBeInTheDocument();
		expect(
			canvas.getByRole("img", { name: "Tool call running" }),
		).toBeVisible();
	},
};

export const Completed: Story = {
	render: () => (
		<ToolCall.Root status="completed" hasContent={false}>
			<ToolCall.Header iconName="read_file" label="Read README.md" />
		</ToolCall.Root>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Read README.md")).toBeVisible();
		expect(
			canvas.queryByRole("img", { name: "Tool call running" }),
		).not.toBeInTheDocument();
	},
};

export const Failed: Story = {
	render: () => (
		<ToolCall.Root
			status="error"
			isError
			errorMessage="Failed to read file"
			hasContent={false}
		>
			<ToolCall.Header iconName="read_file" label="Read README.md" />
		</ToolCall.Root>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Read README.md")).toBeVisible();
		expect(
			canvas.getByRole("img", { name: "Failed to read file" }),
		).toBeVisible();
		expect(
			canvas.queryByRole("img", { name: "Tool call running" }),
		).not.toBeInTheDocument();
	},
};

export const RunningWithBackendError: Story = {
	render: () => (
		<ToolCall.Root
			status="running"
			isError
			errorMessage="Failed to read file"
			hasContent={false}
		>
			<ToolCall.Header iconName="read_file" label="Reading README.md" />
		</ToolCall.Root>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Reading README.md")).toBeVisible();
		expect(
			canvas.getByRole("img", { name: "Tool call running" }),
		).toBeVisible();
		expect(
			canvas.queryByRole("img", { name: "Failed to read file" }),
		).not.toBeInTheDocument();
	},
};

export const Collapsible: Story = {
	render: () => (
		<ToolCall.Root
			status="completed"
			hasContent
			ariaLabel={(expanded) =>
				expanded ? "Collapse read file" : "Expand read file"
			}
		>
			<ToolCall.Header iconName="read_file" label="Read README.md" />
			<ToolCall.Content>
				<div className="mt-1.5 rounded-md border border-solid border-border-default p-3">
					File contents
				</div>
			</ToolCall.Content>
		</ToolCall.Root>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.tab();
		const button = canvas.getByRole("button", { name: "Expand read file" });
		expect(button).toHaveFocus();
		expect(button).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("File contents")).not.toBeInTheDocument();
		await userEvent.keyboard("{Enter}");
		const expandedButton = canvas.getByRole("button", {
			name: "Collapse read file",
		});
		expect(expandedButton).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("File contents")).toBeVisible();
		await userEvent.keyboard(" ");
		expect(
			canvas.getByRole("button", { name: "Expand read file" }),
		).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("File contents")).not.toBeInTheDocument();
	},
};
