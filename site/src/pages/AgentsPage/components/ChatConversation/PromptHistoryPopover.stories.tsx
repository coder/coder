import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import {
	type PromptHistoryEntry,
	PromptHistoryPopover,
} from "./PromptHistoryPopover";

const sampleEntries: readonly PromptHistoryEntry[] = [
	{ id: 1, index: 1, label: "How do I set up a workspace template?" },
	{ id: 2, index: 2, label: "Can you explain the provisioner lifecycle?" },
	{ id: 3, index: 3, label: "Show me how to configure environment variables" },
	{ id: 4, index: 4, label: "What are the best practices for Terraform?" },
	{ id: 5, index: 5, label: "Help me debug this agent connection issue" },
];

const meta: Meta<typeof PromptHistoryPopover> = {
	title: "pages/AgentsPage/ChatConversation/PromptHistoryPopover",
	component: PromptHistoryPopover,
};

export default meta;

type Story = StoryObj<typeof PromptHistoryPopover>;

export const Default: Story = {
	args: {
		entries: sampleEntries,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", { name: "Prompt history" });
		await userEvent.click(trigger);

		const items = await screen.findAllByRole("option");
		await expect(items).toHaveLength(5);
	},
};

export const SearchFiltering: Story = {
	args: {
		entries: sampleEntries,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", { name: "Prompt history" });
		await userEvent.click(trigger);

		const searchInput = await screen.findByRole("combobox");
		await userEvent.type(searchInput, "terraform");

		const items = await screen.findAllByRole("option");
		await expect(items).toHaveLength(1);
		await expect(items[0]).toHaveTextContent("Terraform");
	},
};

export const EmptySearchResults: Story = {
	args: {
		entries: sampleEntries,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", { name: "Prompt history" });
		await userEvent.click(trigger);

		const searchInput = await screen.findByRole("combobox");
		await userEvent.type(searchInput, "xyznonexistent");

		// cmdk renders CommandEmpty when no items match.
		const emptyMessage = await screen.findByText("No matching prompts");
		await expect(emptyMessage).toBeVisible();
	},
};

export const KeyboardNavigation: Story = {
	args: {
		entries: sampleEntries,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", { name: "Prompt history" });
		await userEvent.click(trigger);

		// cmdk auto-selects the first item on open.
		const items = await screen.findAllByRole("option");
		await expect(items[0]).toHaveAttribute("data-selected", "true");

		// ArrowDown moves to the next item.
		await userEvent.keyboard("{ArrowDown}");
		await expect(items[1]).toHaveAttribute("data-selected", "true");

		// ArrowDown again.
		await userEvent.keyboard("{ArrowDown}");
		await expect(items[2]).toHaveAttribute("data-selected", "true");

		// ArrowUp moves back.
		await userEvent.keyboard("{ArrowUp}");
		await expect(items[1]).toHaveAttribute("data-selected", "true");
	},
};
