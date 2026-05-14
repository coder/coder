import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import {
	type PromptHistoryEntry,
	PromptHistoryPopover,
} from "./PromptHistoryPopover";

const sampleEntries: readonly PromptHistoryEntry[] = [
	{ id: 1, index: 1, text: "How do I set up a workspace template?" },
	{ id: 2, index: 2, text: "Can you explain the provisioner lifecycle?" },
	{ id: 3, index: 3, text: "Show me how to configure environment variables" },
	{ id: 4, index: 4, text: "What are the best practices for Terraform?" },
	{ id: 5, index: 5, text: "Help me debug this agent connection issue" },
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

		const listbox = await screen.findByRole("listbox");
		const options = within(listbox).getAllByRole("option");
		await expect(options).toHaveLength(5);
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

		const searchInput = await screen.findByRole("searchbox", {
			name: "Search prompts",
		});
		await userEvent.type(searchInput, "terraform");

		const listbox = await screen.findByRole("listbox");
		const options = within(listbox).getAllByRole("option");
		await expect(options).toHaveLength(1);
		await expect(options[0]).toHaveTextContent("Terraform");

		// Clear search and verify all entries return.
		await userEvent.clear(searchInput);
		const allOptions = within(listbox).getAllByRole("option");
		await expect(allOptions).toHaveLength(5);
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

		const searchInput = await screen.findByRole("searchbox", {
			name: "Search prompts",
		});

		// Press ArrowDown to highlight first item.
		await userEvent.keyboard("{ArrowDown}");
		const listbox = await screen.findByRole("listbox");
		const options = within(listbox).getAllByRole("option");
		await expect(options[0]).toHaveAttribute("aria-selected", "true");

		// Press ArrowDown again to move to second item.
		await userEvent.keyboard("{ArrowDown}");
		await expect(options[0]).toHaveAttribute("aria-selected", "false");
		await expect(options[1]).toHaveAttribute("aria-selected", "true");

		// Verify active-descendant is set on the search input.
		await expect(searchInput).toHaveAttribute(
			"aria-activedescendant",
			`prompt-option-${sampleEntries[1].id}`,
		);
	},
};

export const FewerThanTwoEntries: Story = {
	args: {
		entries: [{ id: 1, index: 1, text: "Only one prompt" }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Component should not render when entries < 2.
		const trigger = canvas.queryByRole("button", { name: "Prompt history" });
		await expect(trigger).toBeNull();
	},
};
